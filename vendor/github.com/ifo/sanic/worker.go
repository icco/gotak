package sanic

import (
	"log"
	"sync"
	"time"
)

type Worker struct {
	ID             int64 // 0 - 2 ^ IDBits
	IDBits         uint64
	IDShift        uint64
	Sequence       int64 // 0 - 2 ^ SequenceBits
	SequenceBits   uint64
	LastTimeStamp  int64
	TimeStampBits  uint64
	TimeStampShift uint64
	Frequency      time.Duration
	TotalBits      uint64
	CustomEpoch    int64
	mutex          sync.Mutex
}

func NewWorker(
	id, epoch int64, idBits, sequenceBits, timestampBits uint64,
	frequency time.Duration) *Worker {

	totalBits := idBits + sequenceBits + timestampBits + 1
	if totalBits%6 != 0 {
		log.Fatal("totalBits + 1 must be evenly divisible by 6")
	}

	w := &Worker{
		ID:             id,
		IDBits:         idBits,
		IDShift:        sequenceBits,
		Sequence:       0,
		SequenceBits:   sequenceBits,
		TimeStampBits:  timestampBits,
		TimeStampShift: sequenceBits + idBits,
		Frequency:      frequency,
		TotalBits:      totalBits,
		CustomEpoch:    epoch,
	}
	// guarantee that the first NextID will start at sequence 0
	w.LastTimeStamp = w.Time() - int64(2*time.Second)
	return w
}

// Easy Worker generation given an ID and using a default configuration
// with a custom epoch of "2016-01-01 00:00:00 +0000 UTC"

// NewWorker10 will generate up to 4096000 unique ids/second for 69 years
// NewWorker10 will return nil if the ID is greater than 63 or less than 0
func NewWorker10(id int64) *Worker {
	if id > 63 || id < 0 { // 2<<6, 6 bits of ID space
		return nil
	}
	return NewWorker(id, 1451606400000, 6, 12, 41, time.Millisecond)
}

// NewWorker9 will generate up to 819200 unique ids/second for 87 years
// NewWorker9 will return nil if the ID is greater than 3 or less than 0
func NewWorker9(id int64) *Worker {
	if id > 3 || id < 0 { // 2<<2, 2 bits of ID space
		return nil
	}
	return NewWorker(id, 145160640000, 2, 13, 38, time.Millisecond*10)
}

// NewWorker8 will generate up to 81920 unique ids/second for 54 years
// NewWorker8 is the only worker of it's size with this configuration
func NewWorker8() *Worker {
	return NewWorker(0, 14516064000, 0, 13, 34, time.Millisecond*100)
}

// NewWorker7 will generate up to 1024 unique ids/second for 68 years
// NewWorker7 is the only worker of its size with this configuration
func NewWorker7() *Worker {
	return NewWorker(0, 1451606400, 0, 10, 31, time.Second)
}

func (w *Worker) NextID() int64 {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	return w.UnsafeNextID()
}

// UnsafeNextID is faster than NextID, but must be called within
// only one goroutine, otherwise ID uniqueness is not guaranteed.
func (w *Worker) UnsafeNextID() int64 {
	timestamp := w.Time()

	if w.LastTimeStamp > timestamp {
		w.waitForNextTime()
	}

	if w.LastTimeStamp == timestamp {
		w.Sequence = (w.Sequence + 1) % (1 << w.SequenceBits)
		if w.Sequence == 0 {
			w.waitForNextTime()
			timestamp = w.LastTimeStamp
		}
	} else {
		w.Sequence = 0
	}

	w.LastTimeStamp = timestamp

	return (timestamp-w.CustomEpoch)<<w.TimeStampShift |
		w.ID<<w.IDShift |
		w.Sequence
}

func (w *Worker) IDString(id int64) string {
	str, _ := IntToString(id, w.TotalBits)
	return str
}

func (w *Worker) waitForNextTime() {
	ts := w.Time()
	for ts <= w.LastTimeStamp {
		ts = w.Time()
	}
	w.LastTimeStamp = ts
}

func (w *Worker) Time() int64 {
	return time.Now().UnixNano() / int64(w.Frequency)
}
