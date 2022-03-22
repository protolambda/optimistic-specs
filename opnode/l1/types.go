package l1

import (
	"encoding/json"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Note: we do this ugly typing because we want the best of both worlds:
// - batched calls of many block requests
// - cached transaction-sender data

type rpcBlock struct {
	Hash         common.Hash      `json:"hash"`
	Transactions []rpcTransaction `json:"transactions"`
	// we ignore the uncles data
}

func (block *rpcBlock) Txs() types.Transactions {
	out := make(types.Transactions, len(block.Transactions))
	for i := 0; i < len(out); i++ {
		tx := block.Transactions[i]
		if tx.From != nil {
			// TODO: priv method :( Worth the diff to expose a public version of this though
			// TODO: trust-model: can we trust the L1 server to supply valid cached Sender data?
			ethclient.setSenderFromServer(tx.tx, *tx.From, block.Hash)
		}
		out[i] = tx.tx
	}
	return out
}

type rpcTransaction struct {
	tx *types.Transaction
	txCacheInfo
}

type txCacheInfo struct {
	// just ignore blockNumber and blockHash extra data
	From *common.Address `json:"from,omitempty"`
}

func (tx *rpcTransaction) UnmarshalJSON(msg []byte) error {
	if err := json.Unmarshal(msg, &tx.tx); err != nil {
		return err
	}
	return json.Unmarshal(msg, &tx.txCacheInfo)
}
