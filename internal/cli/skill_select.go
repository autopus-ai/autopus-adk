package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/insajin/autopus-adk/pkg/skillpolicy"
	"github.com/spf13/cobra"
)

func newSkillSelectCmd() *cobra.Command {
	var policyPath, taskPath, directory, format string
	cmd := &cobra.Command{
		Use: "select", Short: "Select skills using explicit policy and task facts",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "json" {
				return fmt.Errorf("format must be json")
			}
			var policy skillpolicy.Policy
			if err := readSkillPolicyJSON(policyPath, &policy); err != nil {
				return err
			}
			var task skillpolicy.Task
			if err := readSkillPolicyJSON(taskPath, &task); err != nil {
				return err
			}
			result, err := skillpolicy.Select(policy, task, directory)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	cmd.Flags().StringVar(&policyPath, "policy-json", "", "Explicit policy JSON file (required)")
	cmd.Flags().StringVar(&taskPath, "task-json", "", "Explicit task JSON file (required)")
	cmd.Flags().StringVar(&directory, "dir", ".", "Project root for relative file markers")
	cmd.Flags().StringVar(&format, "format", "json", "Output format: json")
	return cmd
}

func readSkillPolicyJSON(path string, target any) error {
	if path == "" {
		return fmt.Errorf("explicit JSON input path is required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("JSON input must be a regular file, not a symlink")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return fmt.Errorf("JSON input changed while opening")
	}
	return skillpolicy.Decode(file, target)
}
