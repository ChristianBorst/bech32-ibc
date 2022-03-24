package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v2/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v2/modules/core/02-client/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the bank MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) banktypes.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ banktypes.MsgServer = msgServer{}

func (k msgServer) Send(goCtx context.Context, msg *banktypes.MsgSend) (*banktypes.MsgSendResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	ctx.Logger().Error("Enter bech32ics20 msg server Send method", "msg", msg)

	if err := k.IsSendEnabledCoins(ctx, msg.Amount...); err != nil {
		return nil, err
	}

	from, err := sdk.AccAddressFromBech32(msg.FromAddress)
	ctx.Logger().Error("Got from address", "from", from)
	if err != nil {
		return nil, err
	}

	prefix, _, err := bech32.DecodeAndConvert(msg.ToAddress)
	ctx.Logger().Error("got to address prefix", "prefix", prefix)
	if err != nil {
		return nil, err
	}

	nativePrefix, err := k.hrpToChannelMapper.GetNativeHrp(ctx)
	ctx.Logger().Error("Got native prefix", "nativePrefix", nativePrefix)
	if err != nil {
		return nil, err
	}

	if prefix == nativePrefix {
		ctx.Logger().Error("Send to local address")
		bankMsgServer := bankkeeper.NewMsgServerImpl(k.Keeper.Keeper)
		return bankMsgServer.Send(goCtx, msg)
	}

	ibcRecord, err := k.hrpToChannelMapper.GetHrpIbcRecord(ctx, prefix)
	ctx.Logger().Error("Got ibc record for to address", "ibcRecord", ibcRecord)
	if err != nil {
		return nil, err
	}

	if msg.Amount.Len() == 0 {
		return nil, sdkerrors.Wrap(banktypes.ErrNoInputs, "invalid send amount")
	}
	if msg.Amount.Len() > 1 {
		return nil, sdkerrors.Wrap(ibctransfertypes.ErrInvalidAmount, "cannot send multiple denoms via IBC")
	}

	portId := k.tk.GetPort(ctx)
	ctx.Logger().Error("got port", "portId", portId)
	_, clientState, err := k.channelKeeper.GetChannelClientState(ctx, portId, ibcRecord.SourceChannel)
	if err != nil {
		return nil, err
	}

	latestHeight := clientState.GetLatestHeight()
	timeoutHeight := clienttypes.NewHeight(latestHeight.GetRevisionNumber(), latestHeight.GetRevisionHeight()+ibcRecord.IcsToHeightOffset)
	ctx.Logger().Error("calculated timeoutHeight", "timeoutHeight", timeoutHeight)

	ibcTransferMsg := ibctransfertypes.NewMsgTransfer(
		portId,
		ibcRecord.SourceChannel,
		msg.Amount[0],
		from.String(),
		msg.ToAddress,
		timeoutHeight, 0, // Use no timeouts for now.  Can add this in future.
	)
	ctx.Logger().Error("created transfer message", "ibcTransferMsg", ibcTransferMsg)

	_, err = k.ics20TransferMsgServer.Transfer(sdk.WrapSDKContext(ctx), ibcTransferMsg)
	ctx.Logger().Error("called Transfer on ics20 msg server", "err", err)

	return &banktypes.MsgSendResponse{}, err
}
func (k msgServer) MultiSend(goCtx context.Context, msg *banktypes.MsgMultiSend) (*banktypes.MsgMultiSendResponse, error) {
	bankMsgServer := bankkeeper.NewMsgServerImpl(k.Keeper.Keeper)
	return bankMsgServer.MultiSend(goCtx, msg)
}
