package db_utils

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent"
	"vpay/internal/db"
)

// WithTx wrap transaction according to doc of Ent
// https://entgo.io/docs/transactions/
func WithTx(ctx context.Context, tx *cep_ent.Tx, fn func(tx *cep_ent.Tx) error) error {
	var err error
	if tx == nil {
		tx, err = db.DB.Tx(ctx)
		if err != nil {
			fmt.Printf("ctx's tx err: %v", err)
			return err
		}
		defer func() {
			// check if anything went wrong
			if v := recover(); v != nil {
				if err = tx.Rollback(); err != nil {
					logrus.Errorf("tx rollback: %v", err)
					return
				}
				panic(v)
			}
		}()
		err = fn(tx)
		// run the process
		if err != nil {
			// process go wrong
			if rErr := tx.Rollback(); rErr != nil {
				// rollback go wrong
				err = fmt.Errorf("%w: rolling back transaction: %v", err, rErr)
			}
			return err
		}
		if err = tx.Commit(); err != nil {
			// commit go wrong
			return fmt.Errorf("committing transaction: %w", err)
		}
		return nil
	} else {
		err = fn(tx)
		return err
	}
}

// WithClient 无事务执行，写成 With 格式而不是 GetClient 是为了方便切换成事务格式时代码结构相似
func WithClient(tx *cep_ent.Tx, fn func(client *cep_ent.Client) error) error {
	if tx == nil {
		// 如果传入事务为空，那么使用全局的 DB Client 作为 client 使用
		err := fn(db.DB)
		return err
	} else {
		// 如果传入事务不为空，那么使用该事务中的 Client 作为 client 使用
		err := fn(tx.Client())
		return err
	}
}
