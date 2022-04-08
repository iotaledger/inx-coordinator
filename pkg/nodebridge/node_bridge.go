package nodebridge

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/gohornet/hornet/pkg/keymanager"
	"github.com/gohornet/hornet/pkg/model/hornet"
	"github.com/gohornet/hornet/pkg/model/milestone"
	"github.com/gohornet/inx-coordinator/pkg/coordinator"
	"github.com/iotaledger/hive.go/events"
	inx "github.com/iotaledger/inx/go"
	iotago "github.com/iotaledger/iota.go/v3"
)

type NodeBridge struct {
	Client             inx.INXClient
	ProtocolParameters *inx.ProtocolParameters
	TangleListener     *TangleListener
	Events             *Events

	isSyncedMutex      sync.RWMutex
	latestMilestone    *inx.Milestone
	confirmedMilestone *inx.Milestone
}

type Events struct {
	MessageSolid              *events.Event
	ConfirmedMilestoneChanged *events.Event
}

func INXMessageMetadataCaller(handler interface{}, params ...interface{}) {
	handler.(func(metadata *inx.MessageMetadata))(params[0].(*inx.MessageMetadata))
}

func INXMilestoneCaller(handler interface{}, params ...interface{}) {
	handler.(func(metadata *inx.Milestone))(params[0].(*inx.Milestone))
}

func NewNodeBridge(ctx context.Context, client inx.INXClient) (*NodeBridge, error) {
	fmt.Println("Connecting to node and reading protocol parameters...")

	retryBackoff := func(_ uint) time.Duration {
		return 1 * time.Second
	}

	protocolParams, err := client.ReadProtocolParameters(ctx, &inx.NoParams{}, grpc_retry.WithMax(5), grpc_retry.WithBackoff(retryBackoff))
	if err != nil {
		return nil, err
	}

	nodeStatus, err := client.ReadNodeStatus(ctx, &inx.NoParams{})
	if err != nil {
		return nil, err
	}

	return &NodeBridge{
		Client:             client,
		ProtocolParameters: protocolParams,
		TangleListener:     newTangleListener(),
		Events: &Events{
			MessageSolid:              events.NewEvent(INXMessageMetadataCaller),
			ConfirmedMilestoneChanged: events.NewEvent(INXMilestoneCaller),
		},
		latestMilestone:    nodeStatus.GetLatestMilestone(),
		confirmedMilestone: nodeStatus.GetConfirmedMilestone(),
	}, nil
}

func (n *NodeBridge) DeserializationParameters() *iotago.DeSerializationParameters {
	return &iotago.DeSerializationParameters{
		RentStructure: &iotago.RentStructure{
			VByteCost:    n.ProtocolParameters.RentStructure.GetVByteCost(),
			VBFactorData: iotago.VByteCostFactor(n.ProtocolParameters.RentStructure.GetVByteFactorData()),
			VBFactorKey:  iotago.VByteCostFactor(n.ProtocolParameters.RentStructure.GetVByteFactorKey()),
		},
	}
}

func (n *NodeBridge) MilestonePublicKeyCount() int {
	return int(n.ProtocolParameters.GetMilestonePublicKeyCount())
}

func (n *NodeBridge) KeyManager() *keymanager.KeyManager {
	keyManager := keymanager.New()
	for _, keyRange := range n.ProtocolParameters.GetMilestoneKeyRanges() {
		keyManager.AddKeyRange(keyRange.GetPublicKey(), milestone.Index(keyRange.GetStartIndex()), milestone.Index(keyRange.GetEndIndex()))
	}
	return keyManager
}

func (n *NodeBridge) Start(ctx context.Context) {
	go n.listenToConfirmedMilestone(ctx)
	go n.listenToLatestMilestone(ctx)
	go n.listenToSolidMessages(ctx)
}

func (n *NodeBridge) IsNodeSynced() bool {
	n.isSyncedMutex.RLock()
	defer n.isSyncedMutex.RUnlock()

	if n.confirmedMilestone == nil || n.latestMilestone == nil {
		return false
	}

	return n.latestMilestone.GetMilestoneIndex() == n.confirmedMilestone.GetMilestoneIndex()
}

func (n *NodeBridge) LatestMilestone() *coordinator.LatestMilestone {
	n.isSyncedMutex.RLock()
	defer n.isSyncedMutex.RUnlock()
	return &coordinator.LatestMilestone{
		Index:     milestone.Index(n.latestMilestone.GetMilestoneIndex()),
		Timestamp: uint64(n.latestMilestone.GetMilestoneTimestamp()),
		MessageID: hornet.MessageIDFromArray(n.latestMilestone.GetMessageId().Unwrap()),
	}
}

func (n *NodeBridge) LatestTreasuryOutput() (*coordinator.LatestTreasuryOutput, error) {
	//TODO: inx, new call
	return nil, nil
}

func (n *NodeBridge) ComputeMerkleTreeHash(ctx context.Context, msIndex milestone.Index, msTimestamp uint64, parents hornet.MessageIDs) (*coordinator.MerkleTreeHash, error) {
	req := &inx.WhiteFlagRequest{
		MilestoneIndex:     uint32(msIndex),
		MilestoneTimestamp: uint32(msTimestamp),
		Parents:            parents.ToSliceOfSlices(),
	}

	res, err := n.Client.ComputeWhiteFlag(ctx, req)
	if err != nil {
		return nil, err
	}

	merkleTreeHash := &coordinator.MerkleTreeHash{}
	copy(merkleTreeHash[:], res.GetMilestoneInclusionMerkleRoot())

	return merkleTreeHash, nil
}

func (n *NodeBridge) EmitMessage(ctx context.Context, message *iotago.Message) error {

	msg, err := inx.WrapMessage(message)
	if err != nil {
		return err
	}

	_, err = n.Client.SubmitMessage(ctx, msg)
	if err != nil {
		return err
	}

	return nil
}

func (n *NodeBridge) listenToSolidMessages(ctx context.Context) error {
	c, cancel := context.WithCancel(ctx)
	defer cancel()
	filter := &inx.MessageFilter{}
	stream, err := n.Client.ListenToSolidMessages(c, filter)
	if err != nil {
		return err
	}
	for {
		messageMetadata, err := stream.Recv()
		if err != nil {
			if err == io.EOF || status.Code(err) == codes.Canceled {
				break
			}
			fmt.Printf("listenToSolidMessages: %s\n", err.Error())
			break
		}
		if c.Err() != nil {
			break
		}
		n.processSolidMessage(messageMetadata)
	}
	return nil
}

func (n *NodeBridge) listenToLatestMilestone(ctx context.Context) error {
	c, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := n.Client.ListenToLatestMilestone(c, &inx.NoParams{})
	if err != nil {
		return err
	}
	for {
		milestone, err := stream.Recv()
		if err != nil {
			if err == io.EOF || status.Code(err) == codes.Canceled {
				break
			}
			fmt.Printf("listenToLatestMilestone: %s\n", err.Error())
			break
		}
		if c.Err() != nil {
			break
		}
		n.processLatestMilestone(milestone)
	}
	return nil
}

func (n *NodeBridge) listenToConfirmedMilestone(ctx context.Context) error {
	c, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := n.Client.ListenToConfirmedMilestone(c, &inx.NoParams{})
	if err != nil {
		return err
	}
	for {
		milestone, err := stream.Recv()
		if err != nil {
			if err == io.EOF || status.Code(err) == codes.Canceled {
				break
			}
			fmt.Printf("listenToConfirmedMilestone: %s\n", err.Error())
			break
		}
		if c.Err() != nil {
			break
		}
		n.processConfirmedMilestone(milestone)
	}
	return nil
}

func (n *NodeBridge) processSolidMessage(metadata *inx.MessageMetadata) {
	n.TangleListener.processSolidMessage(metadata)
	n.Events.MessageSolid.Trigger(metadata)
}

func (n *NodeBridge) processLatestMilestone(ms *inx.Milestone) {
	n.isSyncedMutex.Lock()
	n.latestMilestone = ms
	n.isSyncedMutex.Unlock()
}

func (n *NodeBridge) processConfirmedMilestone(ms *inx.Milestone) {
	n.isSyncedMutex.Lock()
	n.confirmedMilestone = ms
	n.isSyncedMutex.Unlock()

	n.TangleListener.processConfirmedMilestone(ms)
	n.Events.ConfirmedMilestoneChanged.Trigger(ms)
}
