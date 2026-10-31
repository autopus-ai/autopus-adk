package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/spec"
)

// newSpecTimingCmd prints where the last review of a SPEC spent its time, from
// the timing evidence `auto spec review` writes into review-receipt.json.
func newSpecTimingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "timing <SPEC-ID>",
		Short: "Show how long the last SPEC review took, per round and provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := spec.ResolveSpecDir(".", args[0])
			if err != nil {
				return fmt.Errorf("SPEC 로드 실패: %w", err)
			}
			return printSpecTiming(cmd.OutOrStdout(), resolved.SpecDir)
		},
	}
}

func printSpecTiming(w io.Writer, specDir string) error {
	data, err := os.ReadFile(filepath.Join(specDir, "review-receipt.json"))
	if err != nil {
		return fmt.Errorf("review receipt 읽기 실패 (먼저 auto spec review를 실행하세요): %w", err)
	}
	var receipt specReviewPromotionReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return fmt.Errorf("review receipt 해석 실패: %w", err)
	}
	if receipt.StartedAt == "" {
		fmt.Fprintf(w, "%s: 이 receipt에는 소요 시간 기록이 없습니다 (이전 버전의 auto가 썼습니다)\n", receipt.SpecID)
		return nil
	}
	profile := receipt.ReviewProfile
	if profile == "" {
		profile = "full"
	}
	fmt.Fprintf(w, "%s (profile %s, verdict %s)\n", receipt.SpecID, profile, receipt.AnalysisVerdict)
	fmt.Fprintf(w, "리뷰 전체: %s, 시작 %s\n", formatSpecReviewDuration(receipt.DurationMS), receipt.StartedAt)
	for _, round := range receipt.Rounds {
		printSpecReviewRoundTiming(w, round)
	}
	return nil
}
