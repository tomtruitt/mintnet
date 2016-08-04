package main

import (
	"github.com/tendermint/go-crypto"
)

// validator (independent of chain)
type Validator struct {
	ID     string        `json:"id"`
	PubKey crypto.PubKey `json:"pub_key"`
	// Chains []string      `json:"chains,omitempty"`
}

// validator set (independent of chains)
type ValidatorSet struct {
	ID         string       `json:"id"`
	Validators []*Validator `json:"validators"`
}

// validator on a chain
type CoreInfo struct {
	Validator *Validator `json:"validator"`
	P2PAddr   string     `json:"p2p_addr"`
	RPCAddr   string     `json:"rpc_addr"`
	Index     int        `json:"index,omitempty"`
}

type BlockchainInfo struct {
	ID         string      `json:"id"`
	ValSetID   string      `json:"val_set_id"`
	Validators []*CoreInfo `json:"validators"`
}
