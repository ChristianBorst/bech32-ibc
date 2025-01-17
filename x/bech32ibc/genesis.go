package bech32ibc

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/bech32-ibc/x/bech32ibc/keeper"
	"github.com/osmosis-labs/bech32-ibc/x/bech32ibc/types"
)

// InitGenesis initializes the capability module's state from a provided genesis
// state.
func InitGenesis(ctx sdk.Context, k keeper.Keeper, genState types.GenesisState) {
	// this line is used by starport scaffolding # genesis/module/init
}

// ExportGenesis returns the capability module's exported genesis.
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	genesis := types.DefaultGenesis()

	// this line is used by starport scaffolding # genesis/module/export

	return genesis
}
