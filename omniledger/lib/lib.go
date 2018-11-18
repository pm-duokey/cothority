package lib

import (
	"bytes"
	"encoding/binary"
	"github.com/dedis/onet"
	"github.com/dedis/onet/network"
	"time"
)

type ChainConfig struct {
	Roster       *onet.Roster
	ShardCount   int
	EpochSize    time.Duration
	Timestamp    time.Time
	ShardRosters []onet.Roster
}

func ChangeRoster(oldRoster, newRoster onet.Roster, oldMap, newMap map[network.ServerIdentityID]bool) (onet.Roster, map[network.ServerIdentityID]bool, map[network.ServerIdentityID]bool, bool) {
	oldList := oldRoster.List
	newList := newRoster.List

	if oldMap == nil {
		oldMap = make(map[network.ServerIdentityID]bool)
		for _, o := range oldList {
			oldMap[o.ID] = true
		}
	}

	// Add new element of newRoster to OldRoster, one at the time
	for _, n := range newList {
		if _, ok := oldMap[n.ID]; !ok {
			oldRoster.List = append(oldRoster.List, n)
			oldMap[n.ID] = true
			return oldRoster, oldMap, newMap, true
		}
	}

	if newMap == nil {
		newMap = make(map[network.ServerIdentityID]bool)
		for _, n := range newList {
			newMap[n.ID] = true
		}
	}

	// Remove old element of oldRoster, one at the time
	for i, o := range oldList {
		if _, ok := newMap[o.ID]; !ok {
			oldRoster.List = append(oldRoster.List[:i], oldRoster.List[i+1:]...)
			return oldRoster, oldMap, newMap, true
		}
	}

	return oldRoster, oldMap, newMap, false
}

func EncodeDuration(d time.Duration) []byte {
	durationInNs := int64(d * time.Nanosecond)
	tBuf := make([]byte, 8)
	binary.PutVarint(tBuf, durationInNs)

	return tBuf
}

func DecodeDuration(dBuf []byte) (time.Duration, error) {
	decoded, err := binary.ReadVarint(bytes.NewBuffer(dBuf))
	if err != nil {
		return time.Duration(0), err
	}

	duration := time.Duration(int64(decoded)) * time.Nanosecond

	return duration, nil
}
