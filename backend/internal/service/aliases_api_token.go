package service

import "yingce/backend/internal/app"

const (
	APITokenScopeRead          = app.APITokenScopeRead
	APITokenScopeCanvasWrite   = app.APITokenScopeCanvasWrite
	APITokenScopeAssetUpload   = app.APITokenScopeAssetUpload
	APITokenScopeTaskWrite     = app.APITokenScopeTaskWrite
	APITokenScopeAgentRun      = app.APITokenScopeAgentRun
	APITokenScopeGenerateImage = app.APITokenScopeGenerateImage
	APITokenScopeGenerateVideo = app.APITokenScopeGenerateVideo
	APITokenScopeGenerateAudio = app.APITokenScopeGenerateAudio
	CLIContractVersion         = app.CLIContractVersion
	CLIMinCLIVersion           = app.CLIMinCLIVersion
)

var DefaultCLIScopes = app.DefaultCLIScopes

var CLIDeprecatedCommands = app.CLIDeprecatedCommands

var FormatAPITokenScopes = app.FormatAPITokenScopes

type (
	CLIAuthorizeRequest = app.CLIAuthorizeRequest
	CLIAuthorizePreview = app.CLIAuthorizePreview
	CLIApproveRequest   = app.CLIApproveRequest
)
