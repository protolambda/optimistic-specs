package l1

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"

	"github.com/ethereum/go-ethereum/core/types"
)

const MaxReceiptRetry = 3

func fetchReceipts(ctx context.Context, log log.Logger, receiptHash common.Hash, txs types.Transactions, getBatch batchCallContextFn) (types.Receipts, error) {
	if len(txs) == 0 {
		if receiptHash != types.EmptyRootHash {
			return nil, fmt.Errorf("no transactions, but got non-empty receipt trie root: %s", receiptHash)
		}
		return nil, nil
	}

	receipts := make([]*types.Receipt, len(txs))
	receiptRequests := make([]rpc.BatchElem, len(txs))
	for i := 0; i < len(txs); i++ {
		receipts[i] = new(types.Receipt)
		receiptRequests[i] = rpc.BatchElem{
			Method: "eth_getTransactionReceipt",
			Args:   []interface{}{txs[i].Hash()},
			Result: &receipts[i], // receipt may become nil, double pointer is intentional
		}
	}

	batchRequest := func(missing []rpc.BatchElem) (out []rpc.BatchElem, err error) {
		if err := getBatch(ctx, missing); err != nil {
			return nil, fmt.Errorf("failed batch-retrieval of receipts: %v", err)
		}
		for _, elem := range missing {
			if elem.Error != nil {
				log.Trace("batch request element failed", "err", elem.Error, "elem", elem.Args[0])
				elem.Error = nil // reset, we'll try this element again
				out = append(out, elem)
				continue
			} else {
				// on reorgs or other cases the receipts may disappear before they can be retrieved. Exit with error.
				if receiptPtr := elem.Result.(**types.Receipt); *receiptPtr == nil {
					return nil, fmt.Errorf("receipt of tx %s returns nil on retrieval", elem.Args[0])
				}
			}
		}
		return out, nil
	}

	// note: we assume we don't have to split the tx list into multiple batch-requests.
	// We only do another batch request if any element failed.
	for j := 0; j < MaxReceiptRetry; j++ {
		var err error
		receiptRequests, err = batchRequest(receiptRequests)
		if err != nil {
			return nil, fmt.Errorf("failed to batch-request receipts: %v", err)
		}
		if len(receiptRequests) == 0 {
			break
		}
		log.Debug("failed to fetch some elements in batch of tx receipt requests, retrying in 20ms")
		select {
		case <-time.After(20 * time.Millisecond):
			continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Sanity-check: external L1-RPC sources are notorious for not returning all receipts,
	// or returning them out-of-order. Verify the receipts against the expected receipt-hash.
	hasher := trie.NewStackTrie(nil)
	computed := types.DeriveSha(types.Receipts(receipts), hasher)
	if receiptHash != computed {
		return nil, fmt.Errorf("failed to fetch list of receipts: expected receipt root %s but computed %s from retrieved receipts", receiptHash, computed)
	}
	return receipts, nil
}
