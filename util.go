package main

import (
	crand "crypto/rand"
	"math/rand"
	"time"

	log "github.com/sirupsen/logrus"
)

//Necessary for padding all messages to multiple of 4 octets
func addPadding(data []byte) (out []byte) {
	length := len(data)
	padding := 4 - (length % 4)
	if padding > 0 {
		empty := make([]byte, padding)
		out = append(data, empty...)
	}
	return out
}

func concatCopyPreAllocate(slices [][]byte) []byte {
	var totalLen int
	for _, s := range slices {
		totalLen += len(s)
	}
	tmp := make([]byte, totalLen)
	var i int
	for _, s := range slices {
		i += copy(tmp[i:], s)
	}
	return tmp
}

func genRandomBytes(size int) (blk []byte, err error) {
	blk = make([]byte, size)
	_, err = crand.Read(blk)
	return
}

func getRefreshTime(attempt int, lifetime uint32) int64 {
	//Reset seed on each call to avoid non-pseudorandom intervals over prolonged usage
	rand.Seed(time.Now().UnixNano())
	t := time.Now()
	//See 11.2.1 of RFC6887
	max := t.Unix() + (5*int64(lifetime))/(1<<(attempt+3))
	min := t.Unix() + (int64(lifetime))/(1<<(attempt+1))
	var interval int64
	if (max - min) > 0 {
		interval = rand.Int63n(max-min) + min
	}
	if interval < 4 {
		interval = t.Unix() + 4
	}
	log.Debug(max, min, max-min)
	log.Debugf("max - current: %d min - current: %d random int: %d, lifetime: %d", max-t.Unix(), min-t.Unix(), interval-t.Unix(), lifetime)
	log.Debugf("Refresh max: %d Refresh min: %d Time now: %d Interval: %d", max, min, t.Unix(), interval)
	return interval
}
