package opencode

import "path/filepath"

// StaleCompletionPluginScripts returns, in sorted order, the group S scripts
// (SPEC-PANERM-001) that the plugin entries of root's opencode.json load:
// exactly the scripts whose entries Update retracts, computed by the same
// retractStalePluginEntries over the on-disk config. The runtime major decides
// V2 as it does for Update, and options pin it the same way. A config that
// effectivePluginConfig rejects returns its error, as Update fails on it. The
// file is never written.
func StaleCompletionPluginScripts(root string, options ...Option) ([]string, error) {
	a := NewWithRoot(root, options...)
	doc, err := readJSONObject(filepath.Join(root, configFile))
	if err != nil {
		return nil, err
	}
	return retractStalePluginEntries(doc, a.isV2(), a.root)
}
