package utils

import (
	"github.com/gohornet/hornet/pkg/model/hornet"
	inx "github.com/iotaledger/inx/go"
)

func INXBlockIDsFromBlockIDs(blockIDs hornet.MessageIDs) []*inx.BlockId {
	result := make([]*inx.BlockId, len(blockIDs))
	for i := range blockIDs {
		result[i] = inx.NewBlockId(blockIDs[i].ToArray())
	}
	return result
}

func BlockIDsFromINXBlockIDs(blockIDs []*inx.BlockId) hornet.MessageIDs {
	result := make([]hornet.MessageID, len(blockIDs))
	for i := range blockIDs {
		result[i] = hornet.MessageIDFromArray(blockIDs[i].Unwrap())
	}
	return result
}
