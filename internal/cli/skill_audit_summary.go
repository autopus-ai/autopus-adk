package cli

// Estimates describe file sizes, never provider billing or session loads.
type skillAuditSummary struct {
	Registered                             int `json:"registered"`
	ConfiguredCompiled                     int `json:"configured_compiled"`
	ConfiguredVisible                      int `json:"configured_visible"`
	LocalFiles                             int `json:"local_files"`
	LocalMetadataBytes                     int `json:"local_metadata_bytes"`
	LocalBodyBytes                         int `json:"local_body_bytes"`
	LocalMetadataTokenEstimate             int `json:"local_metadata_token_estimate"`
	LocalBodyTokenEstimate                 int `json:"local_body_token_estimate"`
	ConfiguredVisibleMetadataBytes         int `json:"configured_visible_metadata_bytes"`
	ConfiguredVisibleMetadataTokenEstimate int `json:"configured_visible_metadata_token_estimate"`
}

func skillAuditTokenEstimate(size int) int { return (size + 3) / 4 }

func summarizeSkillAudit(report skillAuditReport) skillAuditSummary {
	summary := skillAuditSummary{Registered: len(report.Catalog), LocalFiles: len(report.Installed)}
	for _, entry := range report.Catalog {
		if entry.Compiled {
			summary.ConfiguredCompiled++
		}
		if entry.Visible {
			summary.ConfiguredVisible++
			summary.ConfiguredVisibleMetadataBytes += entry.MetadataBytes
			summary.ConfiguredVisibleMetadataTokenEstimate += entry.MetadataTokenEstimate
		}
	}
	for _, file := range report.Installed {
		summary.LocalMetadataBytes += file.MetadataBytes
		summary.LocalBodyBytes += file.BodyBytes
		summary.LocalMetadataTokenEstimate += file.MetadataTokenEstimate
		summary.LocalBodyTokenEstimate += file.BodyTokenEstimate
	}
	return summary
}
