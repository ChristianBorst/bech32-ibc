package test

import (
	"context"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/relayer/relayer"
	bech32ibctypes "github.com/osmosis-labs/bech32-ibc/x/bech32ibc/types"
	bech32ics20types "github.com/osmosis-labs/bech32-ibc/x/bech32ics20/types"
	"github.com/stretchr/testify/require"
)

var (
	bech32ibcChains = []testChain{
		{"ibc-0", 0, gaiaTestConfig},
		{"ibc-1", 1, bech32ibcTestConfig},
	}
)

// QueryHrpIbcRecords queries hrp ibc records
func QueryHrpIbcRecords(c *relayer.Chain) ([]bech32ibctypes.HrpIbcRecord, error) {
	done := c.UseSDKContext()
	done()

	params := &bech32ibctypes.QueryHrpIbcRecordsRequest{}
	queryClient := bech32ibctypes.NewQueryClient(c.CLIContext(0))

	res, err := queryClient.HrpIbcRecords(context.Background(), params)
	if err != nil {
		return nil, err
	}

	return res.HrpIbcRecords, nil
}

func TestBech32IBCStreamingRelayer(t *testing.T) {
	chains := spinUpTestChains(t, bech32ibcChains...)

	var (
		src            = chains.MustGet("ibc-0")
		dst            = chains.MustGet("ibc-1")
		testDenom      = "samoleans"
		testCoin       = sdk.NewCoin(testDenom, sdk.NewInt(1000))
		twoTestCoin    = sdk.NewCoin(testDenom, sdk.NewInt(2000))
		initialDeposit = sdk.NewCoin(sdk.DefaultBondDenom, sdk.NewInt(20000000))
	)

	path, err := genTestPathAndSet(src, dst, "transfer", "transfer")
	require.NoError(t, err)

	// query initial balances to compare against at the end
	srcExpected, err := src.QueryBalance(src.Key)
	require.NoError(t, err)
	dstExpected, err := dst.QueryBalance(dst.Key)
	require.NoError(t, err)

	// create path
	_, err = src.CreateClients(dst)
	require.NoError(t, err)
	testClientPair(t, src, dst)

	_, err = src.CreateOpenConnections(dst, 3, src.GetTimeout())
	require.NoError(t, err)
	testConnectionPair(t, src, dst)

	_, err = src.CreateOpenChannels(dst, 3, src.GetTimeout())
	require.NoError(t, err)
	testChannelPair(t, src, dst)

	// send a couple of transfers to the queue on src
	require.NoError(t, src.SendTransferMsg(dst, testCoin, dst.MustGetAddress().String(), 0, 0))
	require.NoError(t, src.SendTransferMsg(dst, testCoin, dst.MustGetAddress().String(), 0, 0))

	// send a couple of transfers to the queue on dst
	require.NoError(t, dst.SendTransferMsg(src, testCoin, src.MustGetAddress().String(), 0, 0))
	require.NoError(t, dst.SendTransferMsg(src, testCoin, src.MustGetAddress().String(), 0, 0))

	// Native HRP is set to "stake" as part of genesis in `bech32ibc-setup.sh`
	// Send a proposal to connect hrp with channel
	msg, err := govtypes.NewMsgSubmitProposal(
		&bech32ibctypes.UpdateHrpIbcChannelProposal{
			Title:         "set hrp for gaia network",
			Description:   "set hrp for gaia network",
			Hrp:           gaiaTestConfig.accountPrefix,
			SourceChannel: dst.PathEnd.ChannelID, // TODO: is this correct?
		},
		sdk.Coins{initialDeposit},
		dst.MustGetAddress(),
	)
	require.NoError(t, err)
	resp, _, err := dst.SendMsg(msg)
	require.NoError(t, err)

	dst.Log(fmt.Sprintln("MsgSubmitProposal.Response", resp.Logs))

	// approve the proposal
	// TODO: proposal_id should be fetched from above message response
	resp, _, err = dst.SendMsg(govtypes.NewMsgVote(dst.MustGetAddress(), 1, govtypes.OptionYes))
	require.NoError(t, err)

	dst.Log(fmt.Sprintln("MsgVote.Response", resp.Logs))

	// wait for voting period
	dst.WaitForNBlocks(50)

	dst.Log(fmt.Sprintln("Log after 50 blocks"))

	// TODO: check hrp is updated correctly
	hrpRecords, err := QueryHrpIbcRecords(dst)
	require.NoError(t, err)

	dst.Log(fmt.Sprintln("hrpRecords.Response", hrpRecords))

	// TODO: Broadcast `MsgSend` target address set to native chain address via bech32ics20
	// check balance changes
	_, _, err = dst.SendMsg(&bech32ics20types.MsgSend{
		FromAddress: dst.MustGetAddress().String(),
		ToAddress:   dst.MustGetAddress().String(),
		Amount:      sdk.Coins{testCoin},
	})
	require.NoError(t, err)

	// TODO: Broadcast `MsgSend` target address set to gaia address via bech32ics20
	// check balance changes
	_, _, err = dst.SendMsg(&bech32ics20types.MsgSend{
		FromAddress: dst.MustGetAddress().String(),
		ToAddress:   src.MustGetAddress().String(),
		Amount:      sdk.Coins{testCoin},
	})
	require.NoError(t, err)

	// Wait for message inclusion in both chains
	require.NoError(t, dst.WaitForNBlocks(1))

	// start the relayer process in it's own goroutine
	rlyDone, err := relayer.RunStrategy(src, dst, path.MustGetStrategy())
	require.NoError(t, err)

	// Wait for relay message inclusion in both chains
	require.NoError(t, src.WaitForNBlocks(1))
	require.NoError(t, dst.WaitForNBlocks(1))

	// send those tokens from dst back to dst and src back to src
	require.NoError(t, src.SendTransferMsg(dst, twoTestCoin, dst.MustGetAddress().String(), 0, 0))
	require.NoError(t, dst.SendTransferMsg(src, twoTestCoin, src.MustGetAddress().String(), 0, 0))

	// wait for packet processing
	require.NoError(t, dst.WaitForNBlocks(6))

	// kill relayer routine
	rlyDone()

	// check balance on src against expected
	srcGot, err := src.QueryBalance(src.Key)
	require.NoError(t, err)
	require.Equal(t, srcExpected.AmountOf(testDenom).Int64()-4000, srcGot.AmountOf(testDenom).Int64())

	// check balance on dst against expected
	dstGot, err := dst.QueryBalance(dst.Key)
	require.NoError(t, err)
	require.Equal(t, dstExpected.AmountOf(testDenom).Int64()-4000, dstGot.AmountOf(testDenom).Int64())

	// check balance on src against expected
	srcGot, err = src.QueryBalance(src.Key)
	require.NoError(t, err)
	require.Equal(t, srcExpected.AmountOf(testDenom).Int64()-4000, srcGot.AmountOf(testDenom).Int64())

	// check balance on dst against expected
	dstGot, err = dst.QueryBalance(dst.Key)
	require.NoError(t, err)
	require.Equal(t, dstExpected.AmountOf(testDenom).Int64()-4000, dstGot.AmountOf(testDenom).Int64())
}
