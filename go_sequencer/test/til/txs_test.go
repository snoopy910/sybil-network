package til

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
	"tokamak-sybil-resistance/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKeys(t *testing.T) {
	tc := NewContext(0, common.RollupConstMaxL1UserTx)
	usernames := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	tc.generateKeys(usernames)
	debug := false
	if debug {
		for i, username := range usernames {
			fmt.Println(i, username)
			// sk := crypto.FromECDSA(tc.accounts[username].EthSk)
			// fmt.Println("	eth_sk", hex.EncodeToString(sk))
			fmt.Println("	eth_addr", tc.Accounts[username].Addr)
			fmt.Println("	bjj_sk", hex.EncodeToString(tc.Accounts[username].BJJ[:]))
			fmt.Println("	bjj_pub", tc.Accounts[username].BJJ.Public().Compress())
		}
	}
}

func TestGenerateBlocksNoBatches(t *testing.T) {
	set := `
		Type: Blockchain

		CreateAccountDeposit A: 11
		CreateAccountDeposit B: 22

		> block
	`
	tc := NewContext(0, common.RollupConstMaxL1UserTx)
	blocks, err := tc.GenerateBlocks(set)
	require.NoError(t, err)
	assert.Equal(t, 1, len(blocks))
	assert.Equal(t, 0, len(blocks[0].Rollup.Batches))
	assert.Equal(t, 2, len(blocks[0].Rollup.L1UserTxs))
}

func TestGenerateBlocks(t *testing.T) {
	set := `
		Type: Blockchain
	
		CreateAccountDeposit A: 10
		CreateAccountDeposit B: 5
		Deposit A: 6
		CreateAccountDeposit C: 5
		CreateAccountDeposit D: 5

		> batchL1 // batchNum = 1
		> batchL1 // batchNum = 2

		CreateVouch A-B
		CreateVouch B-A
		CreateVouch A-C
		DeleteVouch A-B

		// set new batch
		> batch // batchNum = 3

		> block

		// Exits
		CreateVouch C-D
		Exit A: 5

		> batch // batchNum = 4
		> block
	`
	tc := NewContext(0, common.RollupConstMaxL1UserTx)
	blocks, err := tc.GenerateBlocks(set)
	require.NoError(t, err)
	assert.Equal(t, 2, len(blocks))
	assert.Equal(t, 3, len(blocks[0].Rollup.Batches))
	assert.Equal(t, 5, len(blocks[0].Rollup.L1UserTxs))
	assert.Equal(t, 1, len(blocks[1].Rollup.Batches))

	// Check expected values generated by each line
	// #0: Deposit(1) A: 10
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[0], common.TxTypeCreateAccountDeposit,
		"A", "", big.NewInt(10), nil)
	// #1: Deposit(1) B: 5
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[1], common.TxTypeCreateAccountDeposit,
		"B", "", big.NewInt(5), nil)
	// #2: Deposit(1) A: 16
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[2], common.TxTypeDeposit,
		"A", "", big.NewInt(6), nil)
	// #3: Deposit(1) C: 5
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[3], common.TxTypeCreateAccountDeposit,
		"C", "", big.NewInt(5), nil)
	// #4: Deposit(1) D: 5
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[4], common.TxTypeCreateAccountDeposit,
		"D", "", big.NewInt(5), nil)
	// #5: CreateVouch A-B
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[2].L2Txs[0], common.TxTypeCreateVouch, "A",
		"B", nil, common.BatchNum(3))
	// #6: CreateVouch B-A
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[2].L2Txs[1], common.TxTypeCreateVouch, "B",
		"A", nil, common.BatchNum(3))
	// #7: CreateVouch A-C
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[2].L2Txs[2], common.TxTypeCreateVouch, "A",
		"C", nil, common.BatchNum(3))
	// #8: CreateVouch A-B
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[2].L2Txs[3], common.TxTypeDeleteVouch, "A",
		"B", nil, common.BatchNum(3))
	// #9: CreateVouch C-D
	tc.checkL2TxParams(t, blocks[1].Rollup.Batches[0].L2Txs[0], common.TxTypeCreateVouch, "C",
		"D", nil, common.BatchNum(4))
	// #9: Exit A: 5
	tc.checkL2TxParams(t, blocks[1].Rollup.Batches[0].L2Txs[1], common.TxTypeExit, "A",
		"", big.NewInt(5), common.BatchNum(4))
}

func (tc *Context) checkL1TxParams(t *testing.T, tx common.L1Tx, typ common.TxType,
	from, to string, depositAmount, amount *big.Int) {
	assert.Equal(t, typ, tx.Type)
	if tx.FromIdx != common.AccountIdx(0) {
		assert.Equal(t, tc.Accounts[from].Idx, tx.FromIdx)
	}
	assert.Equal(t, tc.Accounts[from].Addr.Hex(), tx.FromEthAddr.Hex())
	assert.Equal(t, tc.Accounts[from].BJJ.Public().Compress(), tx.FromBJJ)
	if tx.ToIdx != common.AccountIdx(0) {
		assert.Equal(t, tc.Accounts[to].Idx, tx.ToIdx)
	}
	if depositAmount != nil {
		assert.Equal(t, depositAmount, tx.DepositAmount)
	}
	if amount != nil {
		assert.Equal(t, amount, tx.Amount)
	}
}

func (tc *Context) checkL2TxParams(t *testing.T, tx common.L2Tx, typ common.TxType,
	from, to string, amount *big.Int, batchNum common.BatchNum) {
	assert.Equal(t, typ, tx.Type)
	assert.Equal(t, tc.Accounts[from].Idx, tx.FromIdx)
	if tx.Type != common.TxTypeExit {
		assert.Equal(t, tc.Accounts[to].Idx, tx.ToIdx)
	}
	if amount != nil {
		assert.Equal(t, amount, tx.Amount)
	}
	assert.Equal(t, batchNum, tx.BatchNum)
}
