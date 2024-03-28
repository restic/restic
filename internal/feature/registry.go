package feature

// Flag is named such that checking for a feature uses `feature.Flag.Enabled(feature.ExampleFeature)`.
var Flag = New()

// flag names are written in kebab-case
const (
	DeprecateLegacyIndex FlagName = "deprecate-legacy-index"
	DeviceIDForHardlinks FlagName = "device-id-for-hardlinks"
)

func init() {
	Flag.SetFlags(map[FlagName]FlagDesc{
		DeprecateLegacyIndex: {Type: Beta, Description: "disable support for index format used by restic 0.1.0. Use `restic repair index` to update the index if necessary."},
		DeviceIDForHardlinks: {Type: Alpha, Description: "store deviceID only for hardlinks to reduce metadata changes for example when using btrfs subvolumes. Will be removed in a future restic version after repository format 3 is available"},
	})
}
