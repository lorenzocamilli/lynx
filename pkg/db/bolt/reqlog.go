package bolt

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"

	"github.com/oklog/ulid"
	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"

	"github.com/lorenzocamilli/lynx/pkg/reqlog"
	"github.com/lorenzocamilli/lynx/pkg/scope"
)

var ErrRequestLogsBucketNotFound = errors.New("bolt: request logs bucket not found")

var reqLogsBucketName = []byte("request_logs")

func requestLogsBucket(tx *bolt.Tx, projectID ulid.ULID) (*bolt.Bucket, error) {
	pb, err := projectBucket(tx, projectID[:])
	if err != nil {
		return nil, err
	}

	b := pb.Bucket(reqLogsBucketName)
	if b == nil {
		return nil, ErrRequestLogsBucketNotFound
	}

	return b, nil
}

func (db *Database) FindRequestLogs(ctx context.Context, filter reqlog.FindRequestsFilter, scope *scope.Scope) (reqLogs []reqlog.RequestLog, err error) {
	if filter.ProjectID.Compare(ulid.ULID{}) == 0 {
		return nil, reqlog.ErrProjectIDMustBeSet
	}

	tx, err := db.bolt.Begin(false)
	if err != nil {
		return nil, fmt.Errorf("bolt: failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	b, err := requestLogsBucket(tx, filter.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("bolt: failed to get request logs bucket: %w", err)
	}

	c := b.Cursor()
	skipped := 0

	for k, v := c.Last(); k != nil; k, v = c.Prev() {
		var reqLog reqlog.RequestLog
		if err = gob.NewDecoder(bytes.NewReader(v)).Decode(&reqLog); err != nil {
			return nil, fmt.Errorf("bolt: failed to decode request log: %w", err)
		}

		if filter.OnlyInScope && !reqLog.MatchScope(scope) {
			continue
		}

		if filter.SearchExpr != nil {
			match, err := reqLog.Matches(filter.SearchExpr)
			if err != nil {
				return nil, fmt.Errorf("bolt: failed to match search expression for request log (id: %v): %w", k, err)
			}

			if !match {
				continue
			}
		}

		if skipped < filter.Offset {
			skipped++
			continue
		}

		reqLogs = append(reqLogs, reqLog)

		if filter.Limit > 0 && len(reqLogs) >= filter.Limit {
			break
		}
	}

	return reqLogs, nil
}

func (db *Database) CountRequestLogs(ctx context.Context, projectID ulid.ULID) (int, error) {
	tx, err := db.bolt.Begin(false)
	if err != nil {
		return 0, fmt.Errorf("bolt: failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	b, err := requestLogsBucket(tx, projectID)
	if err != nil {
		return 0, fmt.Errorf("bolt: failed to get request logs bucket: %w", err)
	}

	return b.Stats().KeyN, nil
}

func (db *Database) FindRequestLogByID(ctx context.Context, projectID, reqLogID ulid.ULID) (reqLog reqlog.RequestLog, err error) {
	err = db.bolt.View(func(tx *bolt.Tx) error {
		b, err := requestLogsBucket(tx, projectID)
		if err != nil {
			return fmt.Errorf("bolt: failed to get request logs bucket: %w", err)
		}
		rawReqLog := b.Get(reqLogID[:])
		if rawReqLog == nil {
			return reqlog.ErrRequestNotFound
		}

		err = gob.NewDecoder(bytes.NewReader(rawReqLog)).Decode(&reqLog)
		if err != nil {
			return fmt.Errorf("failed to decode request log: %w", err)
		}

		return nil
	})
	if err != nil {
		return reqlog.RequestLog{}, fmt.Errorf("bolt: failed to find request log by ID: %w", err)
	}

	return reqLog, nil
}

func (db *Database) StoreRequestLog(ctx context.Context, reqLog reqlog.RequestLog) error {
	buf := bytes.Buffer{}

	err := gob.NewEncoder(&buf).Encode(reqLog)
	if err != nil {
		return fmt.Errorf("bolt: failed to encode request log: %w", err)
	}

	err = db.bolt.Update(func(txn *bolt.Tx) error {
		b, err := requestLogsBucket(txn, reqLog.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to get request logs bucket: %w", err)
		}

		err = b.Put(reqLog.ID[:], buf.Bytes())
		if err != nil {
			return fmt.Errorf("failed to put request log: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("bolt: failed to commit transaction: %w", err)
	}

	return nil
}

func (db *Database) StoreResponseLog(ctx context.Context, projectID, reqLogID ulid.ULID, resLog reqlog.ResponseLog) error {
	buf := bytes.Buffer{}

	err := gob.NewEncoder(&buf).Encode(resLog)
	if err != nil {
		return fmt.Errorf("bolt: failed to encode response log: %w", err)
	}

	err = db.bolt.Update(func(txn *bolt.Tx) error {
		b, err := requestLogsBucket(txn, projectID)
		if err != nil {
			return fmt.Errorf("failed to get request logs bucket: %w", err)
		}

		rawReqLog := b.Get(reqLogID[:])
		if rawReqLog == nil {
			return reqlog.ErrRequestNotFound
		}

		var reqLog reqlog.RequestLog
		err = gob.NewDecoder(bytes.NewReader(rawReqLog)).Decode(&reqLog)
		if err != nil {
			return fmt.Errorf("failed to decode request log: %w", err)
		}

		reqLog.Response = &resLog

		buf := bytes.Buffer{}
		err = gob.NewEncoder(&buf).Encode(reqLog)
		if err != nil {
			return fmt.Errorf("failed to encode request log: %w", err)
		}

		err = b.Put(reqLog.ID[:], buf.Bytes())
		if err != nil {
			return fmt.Errorf("failed to put request log: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("bolt: failed to commit transaction: %w", err)
	}

	return nil
}

func (db *Database) DeleteRequestLog(ctx context.Context, projectID, id ulid.ULID) error {
	err := db.bolt.Update(func(tx *bolt.Tx) error {
		b, err := requestLogsBucket(tx, projectID)
		if err != nil {
			return fmt.Errorf("failed to get request logs bucket: %w", err)
		}

		if err := b.Delete(id[:]); err != nil {
			return fmt.Errorf("failed to delete request log: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("bolt: failed to commit transaction: %w", err)
	}

	return nil
}

func (db *Database) ClearRequestLogs(ctx context.Context, projectID ulid.ULID) error {
	err := db.bolt.Update(func(txn *bolt.Tx) error {
		pb, err := projectBucket(txn, projectID[:])
		if err != nil {
			return fmt.Errorf("failed to get project bucket: %w", err)
		}

		if err := pb.DeleteBucket(reqLogsBucketName); err != nil && !errors.Is(err, bolterrors.ErrBucketNotFound) {
			return err
		}

		_, err = pb.CreateBucket(reqLogsBucketName)
		return err
	})
	if err != nil {
		return fmt.Errorf("bolt: failed to commit transaction: %w", err)
	}

	return nil
}
