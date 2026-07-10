package bolt

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"

	"github.com/oklog/ulid"
	bolt "go.etcd.io/bbolt"

	"github.com/lorenzocamilli/lynx/pkg/scope"
	"github.com/lorenzocamilli/lynx/pkg/sender"
)

var ErrSenderRequestsBucketNotFound = errors.New("bolt: sender requests bucket not found")

var senderReqsBucketName = []byte("sender_requests")

func senderReqsBucket(tx *bolt.Tx, projectID ulid.ULID) (*bolt.Bucket, error) {
	pb, err := projectBucket(tx, projectID[:])
	if err != nil {
		return nil, err
	}

	b := pb.Bucket(senderReqsBucketName)
	if b == nil {
		return nil, ErrSenderRequestsBucketNotFound
	}

	return b, nil
}

func (db *Database) StoreSenderRequest(ctx context.Context, req sender.Request) error {
	buf := bytes.Buffer{}

	err := gob.NewEncoder(&buf).Encode(req)
	if err != nil {
		return fmt.Errorf("bolt: failed to encode sender request: %w", err)
	}

	err = db.bolt.Update(func(tx *bolt.Tx) error {
		senderReqsBucket, err := senderReqsBucket(tx, req.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to get sender requests bucket: %w", err)
		}

		err = senderReqsBucket.Put(req.ID[:], buf.Bytes())
		if err != nil {
			return fmt.Errorf("failed to put sender request: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("bolt: failed to commit transaction: %w", err)
	}

	return nil
}

func (db *Database) FindSenderRequestByID(ctx context.Context, projectID, senderReqID ulid.ULID) (req sender.Request, err error) {
	if projectID.Compare(ulid.ULID{}) == 0 {
		return sender.Request{}, sender.ErrProjectIDMustBeSet
	}

	err = db.bolt.View(func(tx *bolt.Tx) error {
		senderReqsBucket, err := senderReqsBucket(tx, projectID)
		if err != nil {
			return fmt.Errorf("failed to get sender requests bucket: %w", err)
		}

		rawSenderReq := senderReqsBucket.Get(senderReqID[:])
		if rawSenderReq == nil {
			return sender.ErrRequestNotFound
		}

		err = gob.NewDecoder(bytes.NewReader(rawSenderReq)).Decode(&req)
		if err != nil {
			return fmt.Errorf("failed to decode sender request: %w", err)
		}

		return nil
	})
	if err != nil {
		return sender.Request{}, fmt.Errorf("bolt: failed to commit transaction: %w", err)
	}

	return req, nil
}

func (db *Database) FindSenderRequests(ctx context.Context, filter sender.FindRequestsFilter, scope *scope.Scope) (reqs []sender.Request, err error) {
	if filter.ProjectID.Compare(ulid.ULID{}) == 0 {
		return nil, sender.ErrProjectIDMustBeSet
	}

	tx, err := db.bolt.Begin(false)
	if err != nil {
		return nil, fmt.Errorf("bolt: failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	b, err := senderReqsBucket(tx, filter.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sender requests bucket: %w", err)
	}

	c := b.Cursor()
	skipped := 0

	for k, v := c.Last(); k != nil; k, v = c.Prev() {
		var req sender.Request
		if err = gob.NewDecoder(bytes.NewReader(v)).Decode(&req); err != nil {
			return nil, fmt.Errorf("bolt: failed to decode sender request: %w", err)
		}

		if filter.OnlyInScope && !req.MatchScope(scope) {
			continue
		}

		if filter.SearchExpr != nil {
			match, err := req.Matches(filter.SearchExpr)
			if err != nil {
				return nil, fmt.Errorf(
					"bolt: failed to match search expression for sender request (id: %v): %w",
					k, err,
				)
			}

			if !match {
				continue
			}
		}

		if skipped < filter.Offset {
			skipped++
			continue
		}

		reqs = append(reqs, req)

		if filter.Limit > 0 && len(reqs) >= filter.Limit {
			break
		}
	}

	return reqs, nil
}

func (db *Database) DeleteSenderRequest(ctx context.Context, projectID, id ulid.ULID) error {
	err := db.bolt.Update(func(tx *bolt.Tx) error {
		b, err := senderReqsBucket(tx, projectID)
		if err != nil {
			return fmt.Errorf("failed to get sender requests bucket: %w", err)
		}

		if err := b.Delete(id[:]); err != nil {
			return fmt.Errorf("failed to delete sender request: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("bolt: failed to commit transaction: %w", err)
	}

	return nil
}

func (db *Database) DeleteSenderRequests(ctx context.Context, projectID ulid.ULID) error {
	err := db.bolt.Update(func(tx *bolt.Tx) error {
		senderReqsBucket, err := senderReqsBucket(tx, projectID)
		if err != nil {
			return fmt.Errorf("failed to get sender requests bucket: %w", err)
		}

		err = senderReqsBucket.DeleteBucket(senderReqsBucketName)
		if err != nil {
			return fmt.Errorf("failed to delete sender requests bucket: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("bolt: failed to commit transaction: %w", err)
	}

	return nil
}
