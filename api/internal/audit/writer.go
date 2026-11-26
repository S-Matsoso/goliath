package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/S-Matsoso/goliath/internal/db"
	"gorm.io/gorm"
)

// Event is the auditable result of an authenticated request.
type Event struct {
	OfficialID *uint64
	Department string
	Action     string
	Subject    []byte
	Purpose    *string
	Outcome    db.AuditOutcome
	RequestID  string
}

// Writer appends serialized rows to one node's hash chain.
type Writer struct {
	database *gorm.DB
	chainID  string
	mu       sync.Mutex
	seq      uint64
	previous []byte
}

// New loads the current chain head before accepting audit writes.
func New(ctx context.Context, database *gorm.DB, chainID string) (*Writer, error) {
	writer := &Writer{database: database, chainID: chainID, previous: make([]byte, sha256.Size)}
	var head db.AuditLog
	err := database.WithContext(ctx).
		Select("seq", "row_hash").
		Where("chain_id = ?", chainID).
		Order("seq DESC").
		First(&head).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load audit chain head: %w", err)
	}
	if err == nil {
		writer.seq = head.Seq
		writer.previous = append([]byte(nil), head.RowHash...)
	}
	return writer, nil
}

// Write appends an event and advances the chain only after the insert succeeds.
func (w *Writer) Write(ctx context.Context, event Event) error {
	return w.InTransaction(ctx, event, nil)
}

// InTransaction atomically commits an operation with its audit event.
func (w *Writer) InTransaction(ctx context.Context, event Event, operation func(*gorm.DB) error) error {
	return w.InTransactionOn(ctx, w.database, event, operation)
}

func (w *Writer) InTransactionOn(ctx context.Context, database *gorm.DB, event Event, operation func(*gorm.DB) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	row := w.newRow(event)
	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if operation != nil {
			if err := operation(tx); err != nil {
				return err
			}
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	w.seq = row.Seq
	w.previous = append(w.previous[:0], row.RowHash...)
	return nil
}

func (w *Writer) WithPreparedEvent(ctx context.Context, event Event, operation func(db.AuditLog) error) error {
	if operation == nil {
		return errors.New("audited operation callback is required")
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	row := w.newRow(event)
	if err := operation(row); err != nil {
		return fmt.Errorf("commit audited operation: %w", err)
	}
	w.seq = row.Seq
	w.previous = append(w.previous[:0], row.RowHash...)
	return nil
}

func (w *Writer) newRow(event Event) db.AuditLog {
	row := db.AuditLog{
		ChainID:      w.chainID,
		Seq:          w.seq + 1,
		OccurredAt:   time.Now().UTC(),
		OfficialID:   event.OfficialID,
		Department:   event.Department,
		Action:       event.Action,
		SubjectIndex: append([]byte(nil), event.Subject...),
		Purpose:      event.Purpose,
		Outcome:      event.Outcome,
		RequestID:    event.RequestID,
		PreviousHash: append([]byte(nil), w.previous...),
	}
	row.RowHash = hash(row)
	return row
}

func hash(row db.AuditLog) []byte {
	values := []string{
		row.ChainID,
		strconv.FormatUint(row.Seq, 10),
		row.OccurredAt.UTC().Format(time.RFC3339Nano),
		optionalUint(row.OfficialID),
		row.Department,
		row.Action,
		hex.EncodeToString(row.SubjectIndex),
		optionalString(row.Purpose),
		string(row.Outcome),
		row.RequestID,
		hex.EncodeToString(row.PreviousHash),
	}
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return sum[:]
}

func optionalUint(value *uint64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatUint(*value, 10)
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
