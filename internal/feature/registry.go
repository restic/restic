package feature

// Flag is named such that checking for a feature uses `feature.Flag.Enabled(feature.ExampleFeature)`.
var Flag = New()

// flag names are written in kebab-case
const (
	BackendErrorRedesign    FlagName = "backend-error-redesign"
	DeviceIDForHardlinks    FlagName = "device-id-for-hardlinks"
	ExplicitS3AnonymousAuth FlagName = "explicit-s3-anonymous-auth"
	SafeForgetKeepTags      FlagName = "safe-forget-keep-tags"
	S3Restore               FlagName = "s3-restore"
)

func init() {
	Flag.SetFlags(map[FlagName]FlagDesc{
		BackendErrorRedesign:    {Type: Beta, Description: "enforce timeouts for stuck HTTP requests and use new backend error handling design."},
		DeviceIDForHardlinks:    {Type: Alpha, Description: "store deviceID only for hardlinks to reduce metadata changes for example when using btrfs subvolumes. Will be removed in a future restic version after repository format 3 is available"},
		ExplicitS3AnonymousAuth: {Type: Stable, Description: "forbid anonymous S3 authentication unless `-o s3.unsafe-anonymous-auth=true` is set"},
		SafeForgetKeepTags:      {Type: Stable, Description: "prevent deleting all snapshots if the tag passed to `forget --keep-tags tagname` does not exist"},
		S3Restore:               {Type: Alpha, Description: "restore S3 objects from cold storage classes when `-o s3.enable-restore=true` is set"},
	})
}
