package rpc

import (
	infuraNode "github.com/scroll-tech/rpc-gateway/node"
	"github.com/scroll-tech/rpc-gateway/rpc/handler"
	"github.com/scroll-tech/rpc-gateway/util/rate"
	"github.com/scroll-tech/rpc-gateway/util/rpc"
	"github.com/sirupsen/logrus"
)

const (
	nativeSpaceRpcServerName = "core_space_rpc"
	evmSpaceRpcServerName    = "evm_space_rpc"

	nativeSpaceBridgeRpcServerName = "core_space_bridge_rpc"
)

// MustNewNativeSpaceServer new core space RPC server by specifying router, handler
// and exposed modules.  Argument exposedModules is a list of API modules to expose
// via the RPC interface. If the module list is empty, all RPC API endpoints designated
// public will be exposed.
func MustNewNativeSpaceServer(
	router infuraNode.Router, gashandler *handler.GasStationHandler,
	exposedModules []string, option ...CfxAPIOption,
) *rpc.Server {
	// retrieve all available core space rpc apis
	clientProvider := infuraNode.NewCfxClientProvider(router)
	allApis := nativeSpaceApis(clientProvider, gashandler, option...)

	exposedApis, err := filterExposedApis(allApis, exposedModules)
	if err != nil {
		logrus.WithError(err).Fatal(
			"Failed to new native space RPC server with bad exposed modules",
		)
	}

	middleware := httpMiddleware(rate.DefaultRegistryCfx, clientProvider)

	return rpc.MustNewServer(nativeSpaceRpcServerName, exposedApis, middleware)
}

// MustNewEvmSpaceServer new evm space RPC server by specifying router, and exposed modules.
// `exposedModules` is a list of API modules to expose via the RPC interface. If the module
// list is empty, all RPC API endpoints designated public will be exposed.
func MustNewEvmSpaceServer(
	router infuraNode.Router, exposedModules []string, option ...EthAPIOption,
) *rpc.Server {
	// retrieve all available evm space rpc apis
	clientProvider := infuraNode.NewEthClientProvider(router)
	allApis, err := evmSpaceApis(clientProvider, option...)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to new EVM space RPC server")
	}

	exposedApis, err := filterExposedApis(allApis, exposedModules)
	if err != nil {
		logrus.WithError(err).Fatal(
			"Failed to new EVM space RPC server with bad exposed modules",
		)
	}

	middleware := httpMiddleware(rate.DefaultRegistryEth, clientProvider)

	return rpc.MustNewServer(evmSpaceRpcServerName, exposedApis, middleware)
}

type CfxBridgeServerConfig struct {
	EthNode        string
	CfxNode        string
	ExposedModules []string
	Endpoint       string `default:":32537"`
}

func MustNewNativeSpaceBridgeServer(config *CfxBridgeServerConfig) *rpc.Server {
	allApis, err := nativeSpaceBridgeApis(config.EthNode, config.CfxNode)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to new CFX bridge RPC server")
	}

	exposedApis, err := filterExposedApis(allApis, config.ExposedModules)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to new CFX bridge RPC server with bad exposed modules")
	}

	return rpc.MustNewServer(nativeSpaceBridgeRpcServerName, exposedApis)
}
