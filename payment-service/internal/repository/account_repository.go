package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"payment-service/internal/domain"
	"time"
)

var ErrAccountNotFound = errors.New("account not found")

type AccountRepository struct {
	db *sql.DB
}

func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) CreateAccount(userID int64) (*domain.Account, error) {
	query := `INSERT INTO accounts (user_id, balance) 
	VALUES ($1, 0)
	ON CONFLICT DO NOTHING
	RETURNING id, user_id, balance, created_at
	`
	acc := &domain.Account{}
	err := r.db.QueryRow(query, userID).Scan(&acc.ID, &acc.UserID, &acc.Balance, &acc.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return r.GetAccountByUserID(userID)
		}
		return nil, fmt.Errorf("create account error: %w", err)
	}
	return acc, nil
}
func (r *AccountRepository) GetAccountByUserID(userID int64) (*domain.Account, error) {
	query := `SELECT id, user_id, balance, created_at FROM accounts WHERE user_id = $1`
	acc := &domain.Account{}
	err := r.db.QueryRow(query, userID).Scan(&acc.ID, &acc.UserID, &acc.Balance, &acc.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAccountNotFound
		}
		return nil, fmt.Errorf("get account error: %w", err)
	}
	return acc, nil
}

func (r *AccountRepository) Deposit(userID int64, amount int64) (*domain.Account, error) {
	if amount < 0 {
		return nil, fmt.Errorf("amount can not be negative: %d", amount)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var accountID int64
	var accountBalance int64
	err = tx.QueryRow(`SELECT id, balance FROM accounts 
                   WHERE user_id = $1 FOR UPDATE`, userID).Scan(&accountID, &accountBalance)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}
	newBalance := amount + accountBalance
	_, err = tx.Exec(`
		UPDATE accounts SET balance = $1 WHERE id = $2`, newBalance, accountID)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`
		INSERT INTO account_transactions (account_id, amount)
		VALUES ($1, $2)`, accountID, amount)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &domain.Account{
		ID:        accountID,
		UserID:    userID,
		Balance:   newBalance,
		CreatedAt: time.Now(),
	}, nil
}
