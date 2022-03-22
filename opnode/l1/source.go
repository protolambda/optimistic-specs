package l1

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	lru "github.com/hashicorp/golang-lru"
	"math/big"

	"github.com/ethereum/go-ethereum/trie"

	"github.com/ethereum-optimism/optimistic-specs/opnode/eth"
	"github.com/ethereum-optimism/optimistic-specs/opnode/rollup/derive"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const MaxBlocksInL1Range = uint64(100)

type batchCallContextFn func(ctx context.Context, b []rpc.BatchElem) error

type Source struct {
	client   *ethclient.Client
	getBatch batchCallContextFn
	log      log.Logger

	// cache receipts in bundles per block hash
	// common.Hash -> types.Receipts
	receiptsCache *lru.Cache

	// cache transactions in bundles per block hash
	// common.Hash -> types.Transactions
	transactionsCache *lru.Cache

	// cache blocks of blocks by hash
	// common.Hash -> *types.Block
	blocksCache *lru.Cache

	// TODO: do we even want to cache blocks by number? Or too fragile to reorgs?

	// cache headers by block hash
	headersCache *lru.Cache
}

func NewSource(client *rpc.Client, log log.Logger) Source {
	// TODO: tune cache sizes
	receiptsCache, _ := lru.New(5000)
	transactionsCache, _ := lru.New(200)
	blocksCache, _ := lru.New(200)
	headersCache, _ := lru.New(200)

	return Source{
		client:            ethclient.NewClient(client),
		getBatch:          client.BatchCallContext,
		log:               log,
		receiptsCache:     receiptsCache,
		transactionsCache: transactionsCache,
		blocksCache:       blocksCache,
		headersCache:      headersCache,
	}
}

func (s *Source) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	return s.client.SubscribeNewHead(ctx, ch)
}

func (s *Source) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	if header, ok := s.headersCache.Get(hash); ok {
		return header.(*types.Header), nil
	}
	header, err := s.client.HeaderByHash(ctx, hash)
	if err == nil {
		s.headersCache.Add(hash, header)
	}
	return header, err
}

func (s *Source) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	// TODO: do we need to cache this? It's small and may suddenly change if the number is recent.
	// And number==nil (head) cannot be cached.
	return s.client.HeaderByNumber(ctx, number)
}

func (s *Source) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	if block, ok := s.blocksCache.Get(hash); ok {
		return block.(*types.Block), nil
	}
	block, err := s.client.BlockByHash(ctx, hash)
	if err == nil {
		s.blocksCache.Add(hash, block)
	}
	return block, err
}

func (s *Source) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	return s.client.BlockByNumber(ctx, number)
}

func (s *Source) Close() {
	s.client.Close()
}

func (s *Source) FetchL1Info(ctx context.Context, id eth.BlockID) (derive.L1Info, error) {
	return s.BlockByHash(ctx, id.Hash)
}

func (s *Source) Fetch(ctx context.Context, id eth.BlockID) (*types.Block, []*types.Receipt, error) {
	block, err := s.BlockByHash(ctx, id.Hash)
	if err != nil {
		return nil, nil, err
	}
	if receipts, ok := s.receiptsCache.Get(id.Hash); ok {
		return block, receipts.(types.Receipts), nil
	}
	receipts, err := fetchReceipts(ctx, s.log, block, s.getBatch)
	if err != nil {
		return nil, nil, err
	}
	s.receiptsCache.Add(id.Hash, receipts)
	return block, receipts, nil
}

func (s *Source) FetchReceipts(ctx context.Context, id eth.BlockID) ([]*types.Receipt, error) {
	_, receipts, err := s.Fetch(ctx, id)
	if err != nil {
		return nil, err
	}
	// Sanity-check: external L1-RPC sources are notorious for not returning all receipts,
	// or returning them out-of-order. Verify the receipts against the expected receipt-hash.
	hasher := trie.NewStackTrie(nil)
	computed := types.DeriveSha(types.Receipts(receipts), hasher)
	if receiptHash != computed {
		return nil, fmt.Errorf("failed to validate receipts of %s, computed receipt-hash %s does not match expected hash %d", id, computed, receiptHash)
	}
	return receipts, nil
}

// FetchAllTransactions fetches transaction lists of a window of blocks, and caches each transaction list per block.
func (s *Source) FetchAllTransactions(ctx context.Context, window []eth.BlockID) ([]types.Transactions, error) {
	// list of transaction lists
	allTxLists := make([]types.Transactions, len(window))

	blockRequests := make([]rpc.BatchElem, 0)
	requestIndices := make([]int, 0)

	for i := 0; i < len(window); i++ {
		txs, ok := s.transactionsCache.Get(window[i].Hash)
		if ok {
			allTxLists[i] = txs.(types.Transactions)
		} else {
			blockRequests = append(blockRequests, rpc.BatchElem{
				Method: "eth_getBlockByHash",
				Args:   []interface{}{window[i].Hash, true}, // request block including transactions list
				Result: new(rpcBlock),
				Error:  nil,
			})
			requestIndices = append(requestIndices, i)
		}
	}
	if err := s.getBatch(ctx, blockRequests); err != nil {
		return nil, err
	}

	// try to cache everything we have before halting on the results with errors
	for i := 0; i < len(blockRequests); i++ {
		if blockRequests[i].Error == nil {
			txs := blockRequests[i].Result.(*rpcBlock).Txs()
			allTxLists[requestIndices[i]] = txs
			// cache the transactions
			s.transactionsCache.Add(blockRequests[i].Args[0], txs)
		}
	}

	for i := 0; i < len(window); i++ {
		if blockRequests[i].Error != nil {
			return nil, fmt.Errorf("failed to retrieve transactions of block %s in batch of %d blocks: %v", blockRequests[i].Args[0], len(blockRequests), blockRequests[i].Error)
		}
	}

	return allTxLists, nil
}

func (s *Source) L1HeadBlockRef(ctx context.Context) (eth.L1BlockRef, error) {
	return s.l1BlockRefByNumber(ctx, nil)
}

func (s *Source) L1BlockRefByNumber(ctx context.Context, l1Num uint64) (eth.L1BlockRef, error) {
	return s.l1BlockRefByNumber(ctx, new(big.Int).SetUint64(l1Num))
}

// l1BlockRefByNumber wraps l1.HeaderByNumber to return an eth.L1BlockRef
// This is internal because the exposed L1BlockRefByNumber takes uint64 instead of big.Ints
func (s *Source) l1BlockRefByNumber(ctx context.Context, number *big.Int) (eth.L1BlockRef, error) {
	header, err := s.HeaderByNumber(ctx, number)
	if err != nil {
		// w%: wrap the error, we still need to detect if a canonical block is not found, a.k.a. end of chain.
		return eth.L1BlockRef{}, fmt.Errorf("failed to determine block-hash of height %v, could not get header: %w", number, err)
	}
	l1Num := header.Number.Uint64()
	parentNum := l1Num
	if parentNum > 0 {
		parentNum -= 1
	}
	return eth.L1BlockRef{
		Self:   eth.BlockID{Hash: header.Hash(), Number: l1Num},
		Parent: eth.BlockID{Hash: header.ParentHash, Number: parentNum},
	}, nil
}

// L1Range returns a range of L1 block beginning just after `begin`.
func (s *Source) L1Range(ctx context.Context, begin eth.BlockID) ([]eth.BlockID, error) {

	// TODO: batch-request
	// TODO: check if a soft API, where we return less if there are errors can work
	// TODO: preseve parent-hash chain checks, blocks need to be chained

	// Ensure that we start on the expected chain.
	if canonicalBegin, err := s.L1BlockRefByNumber(ctx, begin.Number); err != nil {
		return nil, fmt.Errorf("failed to fetch L1 block %v %v: %w", begin.Number, begin.Hash, err)
	} else {
		if canonicalBegin.Self != begin {
			return nil, fmt.Errorf("Re-org at begin block. Expected: %v. Actual: %v", begin, canonicalBegin.Self)
		}
	}

	l1head, err := s.L1HeadBlockRef(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch head L1 block: %w", err)
	}
	maxBlocks := MaxBlocksInL1Range
	// Cap maxBlocks if there are less than maxBlocks between `begin` and the head of the chain.
	if l1head.Self.Number-begin.Number <= maxBlocks {
		maxBlocks = l1head.Self.Number - begin.Number
	}

	if maxBlocks == 0 {
		return nil, nil
	}

	prevHash := begin.Hash
	var res []eth.BlockID
	// TODO: Walk backwards to be able to use block by hash
	for i := begin.Number + 1; i < begin.Number+maxBlocks+1; i++ {
		n, err := s.L1BlockRefByNumber(ctx, i)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch L1 block %v: %w", i, err)
		}
		// TODO(Joshua): Look into why this fails around the genesis block
		if n.Parent.Number != 0 && n.Parent.Hash != prevHash {
			return nil, errors.New("re-organization occurred while attempting to get l1 range")
		}
		prevHash = n.Self.Hash
		res = append(res, n.Self)
	}

	return res, nil
}
