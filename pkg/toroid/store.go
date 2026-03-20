package toroid

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// bucket keys
var (
	bktTraces   = []byte("t")
	bktSpans    = []byte("s")
	bktMeta     = []byte("m")
	bktCosts    = []byte("c")
	bktEvents   = []byte("e")
	bktMemories = []byte("mem")
)

// TraceMeta is stored per trace (root kernel run).
type TraceMeta struct {
	TraceID   string `json:"trace_id"`
	Title     string `json:"title,omitempty"`
	StartedAt int64  `json:"started_at"` // UnixNano
	EndedAt   int64  `json:"ended_at,omitempty"`
}

// SpanMeta is stored per span (kernel session, including subagents).
type SpanMeta struct {
	SpanID       string `json:"span_id"`
	TraceID      string `json:"trace_id"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
	Model        string `json:"model,omitempty"`
	Title        string `json:"title,omitempty"`
	StartedAt    int64  `json:"started_at"` // UnixNano
	EndedAt      int64  `json:"ended_at,omitempty"`
}

// Store wraps a bbolt database for all persistence needs.
type Store struct {
	db *bolt.DB
}

var (
	dbOnce     sync.Once
	defaultDB  *bolt.DB
	defaultDBE error
)

func openDefaultDB() (*bolt.DB, error) {
	dbOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			defaultDBE = err
			return
		}
		dir := filepath.Join(home, ".swb")
		if err := os.MkdirAll(dir, 0755); err != nil {
			defaultDBE = err
			return
		}
		dbPath := filepath.Join(dir, "swb.db")
		db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
		if err != nil {
			defaultDBE = fmt.Errorf("cannot open %s (another swb process may be running): %w", dbPath, err)
			return
		}
		defaultDB = db
	})
	return defaultDB, defaultDBE
}

// NewStore opens (or reuses) the singleton bbolt database.
func NewStore() (*Store, error) {
	db, err := openDefaultDB()
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// SaveTraceMeta writes or updates trace metadata.
func (s *Store) SaveTraceMeta(meta TraceMeta) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		tb, err := tx.CreateBucketIfNotExists(bktTraces)
		if err != nil {
			return err
		}
		trb, err := tb.CreateBucketIfNotExists([]byte(meta.TraceID))
		if err != nil {
			return err
		}
		b, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		return trb.Put(bktMeta, b)
	})
}

// LoadTraceMeta reads trace metadata by trace ID.
func (s *Store) LoadTraceMeta(traceID string) (TraceMeta, error) {
	var meta TraceMeta
	err := s.db.View(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bktTraces)
		if tb == nil {
			return nil
		}
		trb := tb.Bucket([]byte(traceID))
		if trb == nil {
			return nil
		}
		b := trb.Get(bktMeta)
		if b == nil {
			return nil
		}
		return json.Unmarshal(b, &meta)
	})
	return meta, err
}

// SaveSpanMeta writes or updates span metadata.
func (s *Store) SaveSpanMeta(meta SpanMeta) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		tb, err := tx.CreateBucketIfNotExists(bktTraces)
		if err != nil {
			return err
		}
		trb, err := tb.CreateBucketIfNotExists([]byte(meta.TraceID))
		if err != nil {
			return err
		}
		sb, err := trb.CreateBucketIfNotExists(bktSpans)
		if err != nil {
			return err
		}
		spb, err := sb.CreateBucketIfNotExists([]byte(meta.SpanID))
		if err != nil {
			return err
		}
		b, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		return spb.Put(bktMeta, b)
	})
}

// AppendCost records a turn cost under a span's cost bucket.
func (s *Store) AppendCost(traceID, spanID string, turnPaise, totalPaise int64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		tb, err := tx.CreateBucketIfNotExists(bktTraces)
		if err != nil {
			return err
		}
		trb, err := tb.CreateBucketIfNotExists([]byte(traceID))
		if err != nil {
			return err
		}
		sb, err := trb.CreateBucketIfNotExists(bktSpans)
		if err != nil {
			return err
		}
		spb, err := sb.CreateBucketIfNotExists([]byte(spanID))
		if err != nil {
			return err
		}
		cb, err := spb.CreateBucketIfNotExists(bktCosts)
		if err != nil {
			return err
		}
		// key = 8-byte big-endian UnixNano for natural ordering
		var key [8]byte
		binary.BigEndian.PutUint64(key[:], uint64(time.Now().UnixNano()))
		val, err := json.Marshal(map[string]int64{"turn_paise": turnPaise, "total_paise": totalPaise})
		if err != nil {
			return err
		}
		return cb.Put(key[:], val)
	})
}

// AppendEvent records a session event under a span's event bucket.
func (s *Store) AppendEvent(traceID, spanID string, event Event) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		tb, err := tx.CreateBucketIfNotExists(bktTraces)
		if err != nil {
			return err
		}
		trb, err := tb.CreateBucketIfNotExists([]byte(traceID))
		if err != nil {
			return err
		}
		sb, err := trb.CreateBucketIfNotExists(bktSpans)
		if err != nil {
			return err
		}
		spb, err := sb.CreateBucketIfNotExists([]byte(spanID))
		if err != nil {
			return err
		}
		eb, err := spb.CreateBucketIfNotExists(bktEvents)
		if err != nil {
			return err
		}
		// key = 8nd big-endian UnixNano for natural ordering
		var key [8]byte
		binary.BigEndian.PutUint64(key[:], uint64(event.EmitTS))
		val, err := json.Marshal(event)
		if err != nil {
			return err
		}
		return eb.Put(key[:], val)
	})
}

// SaveMemories writes the agent's persistent memory JSON blob for a span.
func (s *Store) SaveMemories(spanID string, mem map[string]any) error {
	b, err := json.MarshalIndent(mem, "", "  ")
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		mb, err := tx.CreateBucketIfNotExists(bktMemories)
		if err != nil {
			return err
		}
		return mb.Put([]byte(spanID), b)
	})
}

// LoadMemories reads the agent's persistent memory JSON blob for a span.
func (s *Store) LoadMemories(spanID string) (map[string]any, error) {
	var mem map[string]any
	err := s.db.View(func(tx *bolt.Tx) error {
		mb := tx.Bucket(bktMemories)
		if mb == nil {
			return nil
		}
		b := mb.Get([]byte(spanID))
		if b == nil {
			return nil
		}
		return json.Unmarshal(b, &mem)
	})
	if mem == nil {
		mem = map[string]any{}
	}
	return mem, err
}

// CostEvent is a single turn cost record stored under a span.
type CostEvent struct {
	TS         int64 `json:"ts"` // UnixNano (from bucket key)
	TurnPaise  int64 `json:"turn_paise"`
	TotalPaise int64 `json:"total_paise"`
}

// SpanData is a span with its cost events and session events, used for visualization.
type SpanData struct {
	SpanMeta
	Costs  []CostEvent `json:"costs"`
	Events []Event     `json:"events"`
}

// TraceData is the full trace for visualization.
type TraceData struct {
	Trace TraceMeta  `json:"trace"`
	Spans []SpanData `json:"spans"`
}

// LoadTraceData reads the full trace + all spans + costs for a given trace ID.
func LoadTraceData(traceID string) (TraceData, error) {
	db, err := openDefaultDB()
	if err != nil {
		return TraceData{}, err
	}
	var td TraceData
	err = db.View(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bktTraces)
		if tb == nil {
			return nil
		}
		trb := tb.Bucket([]byte(traceID))
		if trb == nil {
			return nil
		}
		// trace meta
		if b := trb.Get(bktMeta); b != nil {
			_ = json.Unmarshal(b, &td.Trace)
		}
		// spans
		sb := trb.Bucket(bktSpans)
		if sb == nil {
			return nil
		}
		return sb.ForEach(func(spanKey, v []byte) error {
			if v != nil {
				return nil // skip non-bucket
			}
			spb := sb.Bucket(spanKey)
			if spb == nil {
				return nil
			}
			var sd SpanData
			if b := spb.Get(bktMeta); b != nil {
				_ = json.Unmarshal(b, &sd.SpanMeta)
			}
			// costs
			cb := spb.Bucket(bktCosts)
			if cb != nil {
				_ = cb.ForEach(func(k, v []byte) error {
					ts := int64(binary.BigEndian.Uint64(k))
					var rec map[string]int64
					if err := json.Unmarshal(v, &rec); err == nil {
						sd.Costs = append(sd.Costs, CostEvent{
							TS:         ts,
							TurnPaise:  rec["turn_paise"],
							TotalPaise: rec["total_paise"],
						})
					}
					return nil
				})
			}
			// events
			eb := spb.Bucket(bktEvents)
			if eb != nil {
				_ = eb.ForEach(func(k, v []byte) error {
					var ev Event
					if err := json.Unmarshal(v, &ev); err == nil {
						sd.Events = append(sd.Events, ev)
					}
					return nil
				})
			}
			td.Spans = append(td.Spans, sd)
			return nil
		})
	})
	return td, err
}

// SessionInfo holds metadata for listing traces/sessions.
type SessionInfo struct {
	ID         string // trace ID (root span ID)
	Title      string
	StartedAt  int64 // UnixNano
	DurationNs int64 // last cost event ts - started_at
	TotalPaise int64 // sum of all turn_paise across all spans
}

func (s SessionInfo) StartedAtFmt() string {
	return time.Unix(0, s.StartedAt).Format("Jan 02, 15:04:05")
}

func (s SessionInfo) DurationFmt() string {
	d := time.Duration(s.DurationNs)
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// ListSessions returns all traces sorted newest first.
func ListSessions() ([]SessionInfo, error) {
	db, err := openDefaultDB()
	if err != nil {
		return nil, err
	}
	var infos []SessionInfo
	err = db.View(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bktTraces)
		if tb == nil {
			return nil
		}
		return tb.ForEach(func(k, v []byte) error {
			if v != nil {
				return nil // skip non-bucket entries
			}
			trb := tb.Bucket(k)
			if trb == nil {
				return nil
			}
			b := trb.Get(bktMeta)
			if b == nil {
				return nil
			}
			var meta TraceMeta
			if err := json.Unmarshal(b, &meta); err != nil {
				return nil
			}
			title := meta.Title
			if title == "" {
				title = "(no title)"
			}
			info := SessionInfo{
				ID:        meta.TraceID,
				Title:     title,
				StartedAt: meta.StartedAt,
			}
			// walk all spans to accumulate cost and find last event timestamp
			if sb := trb.Bucket(bktSpans); sb != nil {
				_ = sb.ForEach(func(spanKey, sv []byte) error {
					if sv != nil {
						return nil
					}
					spb := sb.Bucket(spanKey)
					if spb == nil {
						return nil
					}
					cb := spb.Bucket(bktCosts)
					if cb == nil {
						return nil
					}
					return cb.ForEach(func(ck, cv []byte) error {
						ts := int64(binary.BigEndian.Uint64(ck))
						var rec map[string]int64
						if err := json.Unmarshal(cv, &rec); err == nil {
							info.TotalPaise += rec["turn_paise"]
							if d := ts - meta.StartedAt; d > info.DurationNs {
								info.DurationNs = d
							}
						}
						return nil
					})
				})
			}
			infos = append(infos, info)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	// reverse for newest-first (IDs are lexicographically monotonic)
	for i, j := 0, len(infos)-1; i < j; i, j = i+1, j-1 {
		infos[i], infos[j] = infos[j], infos[i]
	}
	return infos, nil
}

// DeleteSession removes all data associated with a trace ID.
func DeleteSession(id string) error {
	db, err := openDefaultDB()
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bktTraces)
		if tb != nil {
			_ = tb.DeleteBucket([]byte(id))
		}
		mb := tx.Bucket(bktMemories)
		if mb != nil {
			_ = mb.Delete([]byte(id))
		}
		return nil
	})
}
