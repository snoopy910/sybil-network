package historydb

import (
	"math/big"
	"tokamak-sybil-resistance/common"
	"tokamak-sybil-resistance/database"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/jmoiron/sqlx"
	"github.com/russross/meddler"
)

// HistoryDB persist the historic of the rollup
type HistoryDB struct {
	dbRead     *sqlx.DB
	dbWrite    *sqlx.DB
	apiConnCon *database.APIConnectionController
}

// NewHistoryDB initialize the DB
func NewHistoryDB(dbRead, dbWrite *sqlx.DB, apiConnCon *database.APIConnectionController) *HistoryDB {
	return &HistoryDB{
		dbRead:     dbRead,
		dbWrite:    dbWrite,
		apiConnCon: apiConnCon,
	}
}

// DB returns a pointer to the L2DB.db. This method should be used only for
// internal testing purposes.
func (hdb *HistoryDB) DB() *sqlx.DB {
	return hdb.dbWrite
}

// AddBlock insert a block into the DB
func (hdb *HistoryDB) AddBlock(block *common.Block) error { return hdb.addBlock(hdb.dbWrite, block) }
func (hdb *HistoryDB) addBlock(d meddler.DB, block *common.Block) error {
	return common.Wrap(meddler.Insert(d, "block", block))
}

// GetBlock retrieve a block from the DB, given a block number
func (hdb *HistoryDB) GetBlock(blockNum int64) (*common.Block, error) {
	block := &common.Block{}
	err := meddler.QueryRow(
		hdb.dbRead, block,
		"SELECT * FROM block WHERE eth_block_num = $1;", blockNum,
	)
	return block, common.Wrap(err)
}

// GetLastBlock retrieve the block with the highest block number from the DB
func (hdb *HistoryDB) GetLastBlock() (*common.Block, error) {
	block := &common.Block{}
	err := meddler.QueryRow(
		hdb.dbRead, block, "SELECT * FROM block ORDER BY eth_block_num DESC LIMIT 1;",
	)
	return block, common.Wrap(err)
}

// getBlocks retrieve blocks from the DB, given a range of block numbers defined by from and to
func (hdb *HistoryDB) getBlocks(from, to int64) ([]common.Block, error) {
	var blocks []*common.Block
	err := meddler.QueryAll(
		hdb.dbRead, &blocks,
		"SELECT * FROM block WHERE $1 <= eth_block_num AND eth_block_num < $2 ORDER BY eth_block_num;",
		from, to,
	)
	return database.SlicePtrsToSlice(blocks).([]common.Block), common.Wrap(err)
}

// GetSCVars returns the rollup, auction and wdelayer smart contracts variables at their last update.
func (hdb *HistoryDB) GetSCVars() (*common.RollupVariables, error) {
	var rollup common.RollupVariables
	if err := meddler.QueryRow(hdb.dbRead, &rollup,
		"SELECT * FROM rollup_vars ORDER BY eth_block_num DESC LIMIT 1;"); err != nil {
		return nil, common.Wrap(err)
	}
	return &rollup, nil
}

// SetInitialSCVars sets the initial state of rollup, auction, wdelayer smart
// contract variables.  This initial state is stored linked to block 0, which
// always exist in the DB and is used to store initialization data that always
// exist in the smart contracts.
func (hdb *HistoryDB) SetInitialSCVars(rollup *common.RollupVariables) error {
	txn, err := hdb.dbWrite.Beginx()
	if err != nil {
		return common.Wrap(err)
	}
	defer func() {
		if err != nil {
			database.Rollback(txn)
		}
	}()
	// Force EthBlockNum to be 0 because it's the block used to link data
	// that belongs to the creation of the smart contracts
	rollup.EthBlockNum = 0
	if err := hdb.setRollupVars(txn, rollup); err != nil {
		return common.Wrap(err)
	}
	return common.Wrap(txn.Commit())
}

func (hdb *HistoryDB) setRollupVars(d meddler.DB, rollup *common.RollupVariables) error {
	return common.Wrap(meddler.Insert(d, "rollup_vars", rollup))
}

// AddCoordinators insert Coordinators into the DB
func (hdb *HistoryDB) AddCoordinators(coordinators []common.Coordinator) error {
	return common.Wrap(hdb.addCoordinators(hdb.dbWrite, coordinators))
}
func (hdb *HistoryDB) addCoordinators(d meddler.DB, coordinators []common.Coordinator) error {
	if len(coordinators) == 0 {
		return nil
	}
	return common.Wrap(database.BulkInsert(
		d,
		"INSERT INTO coordinator (bidder_addr, forger_addr, eth_block_num, url) VALUES %s;",
		coordinators,
	))
}

// GetLastBatch returns the last forged batch
func (hdb *HistoryDB) GetLastBatch() (*common.Batch, error) {
	var batch common.Batch
	err := meddler.QueryRow(
		hdb.dbRead, &batch, `SELECT batch.batch_num, batch.eth_block_num, batch.forger_addr,
		batch.fees_collected, batch.fee_idxs_coordinator, batch.state_root,
		batch.num_accounts, batch.last_idx, batch.exit_root, batch.forge_l1_txs_num,
		batch.slot_num, batch.total_fees_usd, batch.gas_price, batch.gas_used, batch.ether_price_usd
		FROM batch ORDER BY batch_num DESC LIMIT 1;`,
	)
	return &batch, common.Wrap(err)
}

// GetLastL1BatchBlockNum returns the blockNum of the latest forged l1Batch
func (hdb *HistoryDB) GetLastL1BatchBlockNum() (int64, error) {
	row := hdb.dbRead.QueryRow(`SELECT eth_block_num FROM batch
		WHERE forge_l1_txs_num IS NOT NULL
		ORDER BY batch_num DESC LIMIT 1;`)
	var blockNum int64
	return blockNum, common.Wrap(row.Scan(&blockNum))
}

// GetLastL1TxsNum returns the greatest ForgeL1TxsNum in the DB from forged
// batches.  If there's no batch in the DB (nil, nil) is returned.
func (hdb *HistoryDB) GetLastL1TxsNum() (*int64, error) {
	row := hdb.dbRead.QueryRow("SELECT MAX(forge_l1_txs_num) FROM batch;")
	lastL1TxsNum := new(int64)
	return lastL1TxsNum, common.Wrap(row.Scan(&lastL1TxsNum))
}

// AddBatch insert a Batch into the DB
func (hdb *HistoryDB) AddBatch(batch *common.Batch) error { return hdb.addBatch(hdb.dbWrite, batch) }
func (hdb *HistoryDB) addBatch(d meddler.DB, batch *common.Batch) error {
	// Insert to DB
	return common.Wrap(meddler.Insert(d, "batch", batch))
}

// AddBatches insert Bids into the DB
func (hdb *HistoryDB) AddBatches(batches []common.Batch) error {
	return common.Wrap(hdb.addBatches(hdb.dbWrite, batches))
}
func (hdb *HistoryDB) addBatches(d meddler.DB, batches []common.Batch) error {
	for i := 0; i < len(batches); i++ {
		if err := hdb.addBatch(d, &batches[i]); err != nil {
			return common.Wrap(err)
		}
	}
	return nil
}

// GetBatches retrieve batches from the DB, given a range of batch numbers defined by from and to
func (hdb *HistoryDB) GetBatches(from, to common.BatchNum) ([]common.Batch, error) {
	var batches []*common.Batch
	err := meddler.QueryAll(
		hdb.dbRead, &batches,
		`SELECT batch_num, eth_block_num, forger_addr, fees_collected, fee_idxs_coordinator, 
		state_root, num_accounts, last_idx, exit_root, forge_l1_txs_num, slot_num, total_fees_usd, gas_price, gas_used, ether_price_usd 
		FROM batch WHERE $1 <= batch_num AND batch_num < $2 ORDER BY batch_num;`,
		from, to,
	)
	return database.SlicePtrsToSlice(batches).([]common.Batch), common.Wrap(err)
}

// GetLastBatchNum returns the BatchNum of the latest forged batch
func (hdb *HistoryDB) GetLastBatchNum() (common.BatchNum, error) {
	row := hdb.dbRead.QueryRow("SELECT batch_num FROM batch ORDER BY batch_num DESC LIMIT 1;")
	var batchNum common.BatchNum
	return batchNum, common.Wrap(row.Scan(&batchNum))
}

// GetBatch returns the batch with the given batchNum
func (hdb *HistoryDB) GetBatch(batchNum common.BatchNum) (*common.Batch, error) {
	var batch common.Batch
	err := meddler.QueryRow(
		hdb.dbRead, &batch, `SELECT batch.batch_num, batch.eth_block_num, batch.forger_addr,
		batch.fees_collected, batch.fee_idxs_coordinator, batch.state_root,
		batch.num_accounts, batch.last_idx, batch.exit_root, batch.forge_l1_txs_num,
		batch.slot_num, batch.total_fees_usd, batch.gas_price, batch.gas_used, batch.ether_price_usd
		FROM batch WHERE batch_num = $1;`,
		batchNum,
	)
	return &batch, common.Wrap(err)
}

// AddExitTree insert Exit tree into the DB
func (hdb *HistoryDB) AddExitTree(exitTree []common.ExitInfo) error {
	return common.Wrap(hdb.addExitTree(hdb.dbWrite, exitTree))
}
func (hdb *HistoryDB) addExitTree(d meddler.DB, exitTree []common.ExitInfo) error {
	if len(exitTree) == 0 {
		return nil
	}
	return common.Wrap(database.BulkInsert(
		d,
		"INSERT INTO exit_tree (batch_num, account_idx, merkle_proof, balance, "+
			"instant_withdrawn, delayed_withdraw_request, delayed_withdrawn) VALUES %s;",
		exitTree,
	))
}

// AddAccounts insert accounts into the DB
func (hdb *HistoryDB) AddAccounts(accounts []common.Account) error {
	return common.Wrap(hdb.addAccounts(hdb.dbWrite, accounts))
}
func (hdb *HistoryDB) addAccounts(d meddler.DB, accounts []common.Account) error {
	if len(accounts) == 0 {
		return nil
	}
	type TestAccounts struct {
		Idx      common.AccountIdx
		BatchNum common.BatchNum
		BJJ      babyjub.PublicKeyComp
		EthAddr  ethCommon.Address
		Nonce    common.Nonce
		Balance  string
	}

	var testAccounts []TestAccounts

	for _, account := range accounts {
		testAccounts = append(testAccounts, TestAccounts{
			Idx:      account.Idx,
			BatchNum: account.BatchNum,
			BJJ:      account.BJJ,
			EthAddr:  account.EthAddr,
			Nonce:    account.Nonce,
			Balance:  account.Balance.String(),
		})
	}

	return common.Wrap(database.BulkInsert(
		d,
		`INSERT INTO account (
			idx,
			batch_num,
			bjj,
			eth_addr,
			nonce,
			balance
		) VALUES %s;`,
		testAccounts,
	))
}

// AddAccountUpdates inserts accUpdates into the DB
func (hdb *HistoryDB) AddAccountUpdates(accUpdates []common.AccountUpdate) error {
	return common.Wrap(hdb.addAccountUpdates(hdb.dbWrite, accUpdates))
}
func (hdb *HistoryDB) addAccountUpdates(d meddler.DB, accUpdates []common.AccountUpdate) error {
	if len(accUpdates) == 0 {
		return nil
	}
	return common.Wrap(database.BulkInsert(
		d,
		`INSERT INTO account_update (
			eth_block_num,
			batch_num,
			idx,
			nonce,
			balance
		) VALUES %s;`,
		accUpdates,
	))
}

// AddL1Txs inserts L1 txs to the DB. USD and DepositAmountUSD will be set automatically before storing the tx.
// If the tx is originated by a coordinator, BatchNum must be provided. If it's originated by a user,
// BatchNum should be null, and the value will be setted by a trigger when a batch forges the tx.
// EffectiveAmount and EffectiveDepositAmount are seted with default values by the DB.
func (hdb *HistoryDB) AddL1Txs(l1txs []common.L1Tx) error {
	return common.Wrap(hdb.addL1Txs(hdb.dbWrite, l1txs))
}

// addL1Txs inserts L1 txs to the DB. USD and DepositAmountUSD will be set automatically before storing the tx.
// If the tx is originated by a coordinator, BatchNum must be provided. If it's originated by a user,
// BatchNum should be null, and the value will be setted by a trigger when a batch forges the tx.
// EffectiveAmount and EffectiveDepositAmount are seted with default values by the DB.
func (hdb *HistoryDB) addL1Txs(d meddler.DB, l1txs []common.L1Tx) error {
	if len(l1txs) == 0 {
		return nil
	}
	txs := []txWrite{}
	for i := 0; i < len(l1txs); i++ {
		af := new(big.Float).SetInt(l1txs[i].Amount)
		amountFloat, _ := af.Float64()
		laf := new(big.Float).SetInt(l1txs[i].DepositAmount)
		depositAmountFloat, _ := laf.Float64()
		var effectiveFromIdx *common.AccountIdx
		if l1txs[i].UserOrigin {
			if l1txs[i].Type != common.TxTypeCreateAccountDeposit &&
				l1txs[i].Type != common.TxTypeCreateAccountDepositTransfer {
				effectiveFromIdx = &l1txs[i].FromIdx
			}
		} else {
			effectiveFromIdx = &l1txs[i].EffectiveFromIdx
		}
		txs = append(txs, txWrite{
			// Generic
			IsL1:             true,
			TxID:             l1txs[i].TxID,
			Type:             l1txs[i].Type,
			Position:         l1txs[i].Position,
			FromIdx:          &l1txs[i].FromIdx,
			EffectiveFromIdx: effectiveFromIdx,
			ToIdx:            l1txs[i].ToIdx,
			Amount:           l1txs[i].Amount,
			AmountFloat:      amountFloat,
			BatchNum:         l1txs[i].BatchNum,
			EthBlockNum:      l1txs[i].EthBlockNum,
			// L1
			ToForgeL1TxsNum:    l1txs[i].ToForgeL1TxsNum,
			UserOrigin:         &l1txs[i].UserOrigin,
			FromEthAddr:        &l1txs[i].FromEthAddr,
			FromBJJ:            &l1txs[i].FromBJJ,
			DepositAmount:      l1txs[i].DepositAmount,
			DepositAmountFloat: &depositAmountFloat,
			EthTxHash:          &l1txs[i].EthTxHash,
			L1Fee:              l1txs[i].L1Fee,
		})
	}
	return common.Wrap(hdb.addTxs(d, txs))
}

// AddL2Txs inserts L2 txs to the DB. TokenID, USD and FeeUSD will be set automatically before storing the tx.
func (hdb *HistoryDB) AddL2Txs(l2txs []common.L2Tx) error {
	return common.Wrap(hdb.addL2Txs(hdb.dbWrite, l2txs))
}

// addL2Txs inserts L2 txs to the DB. TokenID, USD and FeeUSD will be set automatically before storing the tx.
func (hdb *HistoryDB) addL2Txs(d meddler.DB, l2txs []common.L2Tx) error {
	txs := []txWrite{}
	for i := 0; i < len(l2txs); i++ {
		f := new(big.Float).SetInt(l2txs[i].Amount)
		amountFloat, _ := f.Float64()
		txs = append(txs, txWrite{
			// Generic
			IsL1:             false,
			TxID:             l2txs[i].TxID,
			Type:             l2txs[i].Type,
			Position:         l2txs[i].Position,
			FromIdx:          &l2txs[i].FromIdx,
			EffectiveFromIdx: &l2txs[i].FromIdx,
			ToIdx:            l2txs[i].ToIdx,
			Amount:           l2txs[i].Amount,
			AmountFloat:      amountFloat,
			BatchNum:         &l2txs[i].BatchNum,
			EthBlockNum:      l2txs[i].EthBlockNum,
			// L2
			Nonce: &l2txs[i].Nonce,
		})
	}
	return common.Wrap(hdb.addTxs(d, txs))
}

func (hdb *HistoryDB) addTxs(d meddler.DB, txs []txWrite) error {
	if len(txs) == 0 {
		return nil
	}
	return common.Wrap(database.BulkInsert(
		d,
		`INSERT INTO tx (
			is_l1,
			id,
			type,
			position,
			from_idx,
			effective_from_idx,
			to_idx,
			amount,
			amount_f,
			token_id,
			batch_num,
			eth_block_num,
			to_forge_l1_txs_num,
			user_origin,
			from_eth_addr,
			from_bjj,
			deposit_amount,
			deposit_amount_f,
			eth_tx_hash,
			l1_fee,
			fee,
			nonce
		) VALUES %s;`,
		txs,
	))
}

// setExtraInfoForgedL1UserTxs sets the EffectiveAmount, EffectiveDepositAmount
// and EffectiveFromIdx of the given l1UserTxs (with an UPDATE)
func (hdb *HistoryDB) setExtraInfoForgedL1UserTxs(d sqlx.Ext, txs []common.L1Tx) error {
	if len(txs) == 0 {
		return nil
	}
	// Effective amounts are stored as success flags in the DB, with true value by default
	// to reduce the amount of updates. Therefore, only amounts that became uneffective should be
	// updated to become false.  At the same time, all the txs that contain
	// accounts (FromIdx == 0) are updated to set the EffectiveFromIdx.
	type txUpdate struct {
		ID                   common.TxID       `db:"id"`
		AmountSuccess        bool              `db:"amount_success"`
		DepositAmountSuccess bool              `db:"deposit_amount_success"`
		EffectiveFromIdx     common.AccountIdx `db:"effective_from_idx"`
	}
	txUpdates := []txUpdate{}
	equal := func(a *big.Int, b *big.Int) bool {
		return a.Cmp(b) == 0
	}
	for i := range txs {
		amountSuccess := equal(txs[i].Amount, txs[i].EffectiveAmount)
		depositAmountSuccess := equal(txs[i].DepositAmount, txs[i].EffectiveDepositAmount)
		if !amountSuccess || !depositAmountSuccess || txs[i].FromIdx == 0 {
			txUpdates = append(txUpdates, txUpdate{
				ID:                   txs[i].TxID,
				AmountSuccess:        amountSuccess,
				DepositAmountSuccess: depositAmountSuccess,
				EffectiveFromIdx:     txs[i].EffectiveFromIdx,
			})
		}
	}
	const query string = `
		UPDATE tx SET
			amount_success = tx_update.amount_success,
			deposit_amount_success = tx_update.deposit_amount_success,
			effective_from_idx = tx_update.effective_from_idx
		FROM (VALUES
			(NULL::::BYTEA, NULL::::BOOL, NULL::::BOOL, NULL::::BIGINT),
			(:id, :amount_success, :deposit_amount_success, :effective_from_idx)
		) as tx_update (id, amount_success, deposit_amount_success, effective_from_idx)
		WHERE tx.id = tx_update.id;
	`
	if len(txUpdates) > 0 {
		if _, err := sqlx.NamedExec(d, query, txUpdates); err != nil {
			return common.Wrap(err)
		}
	}
	return nil
}

// AddBlockSCData stores all the information of a block retrieved by the
// Synchronizer.  Blocks should be inserted in order, leaving no gaps because
// the pagination system of the API/DB depends on this.  Within blocks, all
// items should also be in the correct order (Accounts, Tokens, Txs, etc.)
func (hdb *HistoryDB) AddBlockSCData(blockData *common.BlockData) (err error) {
	txn, err := hdb.dbWrite.Beginx()
	if err != nil {
		return common.Wrap(err)
	}
	defer func() {
		if err != nil {
			database.Rollback(txn)
		}
	}()

	// Add block
	if err := hdb.addBlock(txn, &blockData.Block); err != nil {
		return common.Wrap(err)
	}

	// // Add Coordinators
	// if err := hdb.addCoordinators(txn, blockData.Auction.Coordinators); err != nil {
	// 	return common.Wrap(err)
	// }

	// // Add Bids
	// if err := hdb.addBids(txn, blockData.Auction.Bids); err != nil {
	// 	return common.Wrap(err)
	// }

	// // Add Tokens
	// if err := hdb.addTokens(txn, blockData.Rollup.AddedTokens); err != nil {
	// 	return common.Wrap(err)
	// }

	// Prepare user L1 txs to be added.
	// They must be added before the batch that will forge them (which can be in the same block)
	// and after the account that will be sent to (also can be in the same block).
	// Note: insert order is not relevant since item_id will be updated by a DB trigger when
	// the batch that forges those txs is inserted
	userL1s := make(map[common.BatchNum][]common.L1Tx)
	for i := range blockData.Rollup.L1UserTxs {
		batchThatForgesIsInTheBlock := false
		for _, batch := range blockData.Rollup.Batches {
			if batch.Batch.ForgeL1TxsNum != nil &&
				*batch.Batch.ForgeL1TxsNum == *blockData.Rollup.L1UserTxs[i].ToForgeL1TxsNum {
				// Tx is forged in this block. It's guaranteed that:
				// * the first batch of the block won't forge user L1 txs that have been added in this block
				// * batch nums are sequential therefore it's safe to add the tx at batch.BatchNum -1
				batchThatForgesIsInTheBlock = true
				addAtBatchNum := batch.Batch.BatchNum - 1
				userL1s[addAtBatchNum] = append(userL1s[addAtBatchNum], blockData.Rollup.L1UserTxs[i])
				break
			}
		}
		if !batchThatForgesIsInTheBlock {
			// User artificial batchNum 0 to add txs that are not forge in this block
			// after all the accounts of the block have been added
			userL1s[0] = append(userL1s[0], blockData.Rollup.L1UserTxs[i])
		}
	}

	// Add Batches
	for i := range blockData.Rollup.Batches {
		batch := &blockData.Rollup.Batches[i]
		batch.Batch.GasPrice = big.NewInt(0)

		// Add Batch: this will trigger an update on the DB
		// that will set the batch num of forged L1 txs in this batch
		if err = hdb.addBatch(txn, &batch.Batch); err != nil {
			return common.Wrap(err)
		}

		// Add accounts
		if err := hdb.addAccounts(txn, batch.CreatedAccounts); err != nil {
			return common.Wrap(err)
		}

		// Add accountBalances if it exists
		if err := hdb.addAccountUpdates(txn, batch.UpdatedAccounts); err != nil {
			return common.Wrap(err)
		}

		// Set the EffectiveAmount and EffectiveDepositAmount of all the
		// L1UserTxs that have been forged in this batch
		if err = hdb.setExtraInfoForgedL1UserTxs(txn, batch.L1UserTxs); err != nil {
			return common.Wrap(err)
		}

		// Add forged l1 coordinator Txs
		if err := hdb.addL1Txs(txn, batch.L1CoordinatorTxs); err != nil {
			return common.Wrap(err)
		}

		// Add l2 Txs
		if err := hdb.addL2Txs(txn, batch.L2Txs); err != nil {
			return common.Wrap(err)
		}

		// Add user L1 txs that will be forged in next batch
		if userlL1s, ok := userL1s[batch.Batch.BatchNum]; ok {
			if err := hdb.addL1Txs(txn, userlL1s); err != nil {
				return common.Wrap(err)
			}
		}

		// Add exit tree
		if err := hdb.addExitTree(txn, batch.ExitTree); err != nil {
			return common.Wrap(err)
		}
	}
	// Add user L1 txs that won't be forged in this block
	if userL1sNotForgedInThisBlock, ok := userL1s[0]; ok {
		if err := hdb.addL1Txs(txn, userL1sNotForgedInThisBlock); err != nil {
			return common.Wrap(err)
		}
	}

	// Set SC Vars if there was an update
	if blockData.Rollup.Vars != nil {
		if err := hdb.setRollupVars(txn, blockData.Rollup.Vars); err != nil {
			return common.Wrap(err)
		}
	}

	// // Update withdrawals in exit tree table
	// if err := hdb.updateExitTree(txn, blockData.Block.Num,
	// 	blockData.Rollup.Withdrawals, blockData.WDelayer.Withdrawals); err != nil {
	// 	return common.Wrap(err)
	// }

	// // Add Escape Hatch Withdrawals
	// if err := hdb.addEscapeHatchWithdrawals(txn,
	// 	blockData.WDelayer.EscapeHatchWithdrawals); err != nil {
	// 	return common.Wrap(err)
	// }

	// // Add Buckets withdrawals updates
	// if err := hdb.addBucketUpdates(txn, blockData.Rollup.UpdateBucketWithdraw); err != nil {
	// 	return common.Wrap(err)
	// }

	return common.Wrap(txn.Commit())
}
