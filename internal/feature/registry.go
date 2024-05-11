package feature

// Flag is named such that checking for a feature uses `feature.Flag.Enabled(feature.ExampleFeature)`.
var Flag = New()

// flag names are written in kebab-case
const (
	BackendErrorRedesign    FlagName = "backend-error-redesign"
	DeprecateLegacyIndex    FlagName = "deprecate-legacy-index"
	DeprecateS3LegacyLayout FlagName = "deprecate-s3-legacy-layout"
	DeviceIDForHardlinks    FlagName = "device-id-for-hardlinks"
)

func init() {
	Flag.SetFlags(map[FlagName]FlagDesc{
		BackendErrorRedesign:    {Type: Beta, Description: "enforce timeouts for stuck HTTP requests and use new backend error handling design."},
		DeprecateLegacyIndex:    {Type: Beta, Description: "disable support for index format used by restic 0.1.0. Use `restic repair index` to update the index if necessary."},
		DeprecateS3LegacyLayout: {Type: Beta, Description: "disable support for S3 legacy layout used up to restic 0.7.0. Use `RESTIC_FEATURES=deprecate-s3-legacy-layout=false restic migrate s3_layout` to migrate your S3 repository if necessary."},
		DeviceIDForHardlinks:    {Type: Alpha, Description: "store deviceID only for hardlinks to reduce metadata changes for example when using btrfs subvolumes. Will be removed in a future restic version after repository format 3 is available"},
	})
}
