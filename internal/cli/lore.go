package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/lore"
)

// newLoreCmd는 lore 서브커맨드를 생성한다.
func newLoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lore",
		Short: "Decision knowledge management via git trailers",
	}

	cmd.AddCommand(newLoreContextCmd())
	cmd.AddCommand(newLoreConstraintsCmd())
	cmd.AddCommand(newLoreRejectedCmd())
	cmd.AddCommand(newLoreDirectivesCmd())
	cmd.AddCommand(newLoreStaleCmd())
	cmd.AddCommand(newLoreCommitCmd())
	cmd.AddCommand(newLoreValidateCmd())
	return cmd
}

func newLoreContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context <path>",
		Short: "Query lore entries for a file path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := lore.QueryContext(".", args[0])
			if err != nil {
				return err
			}
			printLoreEntries(entries)
			return nil
		},
	}
}

func newLoreConstraintsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "constraints",
		Short: "Query all constraint lore entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := lore.QueryConstraints(".")
			if err != nil {
				return err
			}
			printLoreEntries(entries)
			return nil
		},
	}
}

func newLoreRejectedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rejected",
		Short: "Query all rejected alternatives",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := lore.QueryRejected(".")
			if err != nil {
				return err
			}
			printLoreEntries(entries)
			return nil
		},
	}
}

func newLoreDirectivesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "directives",
		Short: "Query all directive lore entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := lore.QueryDirectives(".")
			if err != nil {
				return err
			}
			printLoreEntries(entries)
			return nil
		},
	}
}

func newLoreStaleCmd() *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Query lore entries older than N days",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := lore.QueryStale(".", days)
			if err != nil {
				return err
			}
			fmt.Printf("%d일 이상 된 항목: %d건\n", days, len(entries))
			printLoreEntries(entries)
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 90, "Stale threshold in days")
	return cmd
}

func newLoreCommitCmd() *cobra.Command {
	var (
		constraint    string
		rejected      string
		confidence    string
		scopeRisk     string
		reversibility string
		directive     string
		tested        string
		notTested     string
		related       string
	)

	cmd := &cobra.Command{
		Use:   "commit <message>",
		Short: "Build a commit message with lore trailers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry := &lore.LoreEntry{
				Constraint:    constraint,
				Rejected:      rejected,
				Confidence:    confidence,
				ScopeRisk:     scopeRisk,
				Reversibility: reversibility,
				Directive:     directive,
				Tested:        tested,
				NotTested:     notTested,
				Related:       related,
			}

			result, err := lore.BuildCommit(entry, args[0])
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&constraint, "constraint", "", "Constraint trailer")
	cmd.Flags().StringVar(&rejected, "rejected", "", "Rejected trailer")
	cmd.Flags().StringVar(&confidence, "confidence", "", "Confidence: low|medium|high")
	cmd.Flags().StringVar(&scopeRisk, "scope-risk", "", "Scope-risk: local|module|system")
	cmd.Flags().StringVar(&reversibility, "reversibility", "", "Reversibility: trivial|moderate|difficult")
	cmd.Flags().StringVar(&directive, "directive", "", "Directive trailer")
	cmd.Flags().StringVar(&tested, "tested", "", "Tested trailer")
	cmd.Flags().StringVar(&notTested, "not-tested", "", "Not-tested trailer")
	cmd.Flags().StringVar(&related, "related", "", "Related trailer")
	return cmd
}

func newLoreValidateCmd() *cobra.Command {
	var requiredTrailers []string
	var staleDays int
	forbiddenTrailers := config.LoreConf{}.EffectiveForbiddenTrailers()

	cmd := &cobra.Command{
		Use:   "validate <commit-message-file>",
		Short: "Validate lore trailers in a commit message",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("파일 읽기 실패: %w", err)
			}

			if !cmd.Flags().Changed("required") || !cmd.Flags().Changed("stale-days") {
				configPathDir := loreConfigDirForMessageFile(args[0])
				cfg, err := config.Load(configPathDir)
				if err != nil {
					return fmt.Errorf("설정 로드 실패: %w", err)
				}
				if !cmd.Flags().Changed("required") {
					requiredTrailers = append([]string(nil), cfg.Lore.RequiredTrailers...)
				}
				if !cmd.Flags().Changed("stale-days") {
					staleDays = cfg.Lore.StaleThresholdDays
				}
				forbiddenTrailers = cfg.Lore.EffectiveForbiddenTrailers()
			}

			loreConfig := lore.LoreConfig{
				RequiredTrailers:   requiredTrailers,
				StaleThresholdDays: staleDays,
				ForbiddenTrailers:  forbiddenTrailers,
			}

			errs := lore.Validate(string(content), loreConfig)
			if len(errs) == 0 {
				fmt.Println("유효한 Lore 커밋 메시지입니다")
				return nil
			}

			fmt.Fprintf(os.Stderr, "검증 오류 %d건:\n", len(errs))
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "  [%s] %s\n", e.Field, e.Message)
			}
			return fmt.Errorf("%d개의 검증 오류", len(errs))
		},
	}

	cmd.Flags().StringSliceVar(&requiredTrailers, "required", nil, "Required trailers (comma-separated)")
	cmd.Flags().IntVar(&staleDays, "stale-days", 90, "Stale threshold in days")
	return cmd
}

func loreConfigDirForMessageFile(messageFile string) string {
	dir := filepath.Dir(messageFile)
	if filepath.Base(dir) == ".git" {
		return filepath.Dir(dir)
	}
	return dir
}

// printLoreEntries는 Lore 항목 목록을 출력한다.
func printLoreEntries(entries []lore.LoreEntry) {
	if len(entries) == 0 {
		fmt.Println("항목 없음")
		return
	}

	for i, e := range entries {
		fmt.Printf("\n[%s] %s\n", strconv.Itoa(i+1), e.CommitMsg)
		if e.Constraint != "" {
			fmt.Printf("  Constraint: %s\n", e.Constraint)
		}
		if e.Rejected != "" {
			fmt.Printf("  Rejected: %s\n", e.Rejected)
		}
		if e.Confidence != "" {
			fmt.Printf("  Confidence: %s\n", e.Confidence)
		}
		if e.ScopeRisk != "" {
			fmt.Printf("  Scope-risk: %s\n", e.ScopeRisk)
		}
		if e.Reversibility != "" {
			fmt.Printf("  Reversibility: %s\n", e.Reversibility)
		}
		if e.Directive != "" {
			fmt.Printf("  Directive: %s\n", e.Directive)
		}
		if e.Tested != "" {
			fmt.Printf("  Tested: %s\n", e.Tested)
		}
		if e.NotTested != "" {
			fmt.Printf("  Not-tested: %s\n", e.NotTested)
		}
		if e.Related != "" {
			fmt.Printf("  Related: %s\n", e.Related)
		}
		if !e.CommitDate.IsZero() {
			fmt.Printf("  Date: %s\n", e.CommitDate.Format("2006-01-02"))
		}
	}
}
