package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/linlay/cli-imagine/internal/buildinfo"
	"github.com/linlay/cli-imagine/internal/config"
	"github.com/linlay/cli-imagine/internal/imagine"
	"github.com/linlay/cli-imagine/internal/verify"
)

type outputFormat string

const (
	formatText outputFormat = "text"
	formatJSON outputFormat = "json"
)

type rootOptions struct {
	ConfigDir string
	Format    outputFormat
}

type jsonArgMap map[string]any
type stringListValue []string

func (v *jsonArgMap) String() string { return "" }

func (v *jsonArgMap) Set(raw string) error {
	key, value, ok := strings.Cut(raw, "=")
	if !ok || strings.TrimSpace(key) == "" {
		return fmt.Errorf("invalid --arg %q, expected key=json", raw)
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return fmt.Errorf("invalid --arg %q: %v", raw, err)
	}
	if *v == nil {
		*v = map[string]any{}
	}
	(*v)[strings.TrimSpace(key)] = decoded
	return nil
}

func (v *stringListValue) String() string {
	if v == nil {
		return ""
	}
	return strings.Join(*v, ",")
}

func (v *stringListValue) Set(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("empty value is not allowed")
	}
	*v = append(*v, trimmed)
	return nil
}

func newRootCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	opts := &rootOptions{ConfigDir: defaultConfigDir(), Format: ""}
	app := &cliApp{stdout: stdout, stderr: stderr}

	root := &cobra.Command{
		Use:           "imagine",
		Short:         "Configurable image generation CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.PersistentFlags().StringVar(&opts.ConfigDir, "config", opts.ConfigDir, "Configuration directory")
	root.PersistentFlags().Var(newFormatValue(&opts.Format), "format", "Output format: text or json")

	root.AddCommand(
		app.newProvidersCommand(opts),
		app.newModelsCommand(opts),
		app.newModelCommand(opts),
		app.newGenerateCommand(opts),
		app.newEditCommand(opts),
		app.newImportCommand(opts),
		app.newRunCommand(opts),
		app.newInspectCommand(opts),
		app.newConfigCommand(opts),
		app.newVerifyCommand(opts),
		newVersionCommand(),
	)
	return root
}

type formatValue struct{ target *outputFormat }

func newFormatValue(target *outputFormat) *formatValue { return &formatValue{target: target} }
func (v *formatValue) String() string {
	if v == nil || v.target == nil {
		return ""
	}
	return string(*v.target)
}
func (v *formatValue) Set(raw string) error {
	switch outputFormat(strings.TrimSpace(raw)) {
	case formatText, formatJSON:
		*v.target = outputFormat(strings.TrimSpace(raw))
		return nil
	default:
		return fmt.Errorf("unsupported format %q", raw)
	}
}

type cliApp struct {
	stdout io.Writer
	stderr io.Writer
}

func (a *cliApp) loadConfig(dir string) (config.Config, error) {
	return config.LoadDir(dir)
}

func (a *cliApp) runtime(cfg config.Config, outputDir string) *imagine.Runtime {
	return imagine.NewRuntime(cfg, imagine.RunContext{OutputDir: outputDir})
}

func (a *cliApp) newProvidersCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List configured providers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig(opts.ConfigDir)
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			items := make([]map[string]any, 0, len(cfg.Providers))
			for _, provider := range cfg.Providers {
				items = append(items, map[string]any{
					"name":        provider.Name,
					"description": provider.Description,
					"path":        provider.Path,
					"models":      len(provider.Models),
				})
			}
			return a.render(cmd.OutOrStdout(), defaultFormat(opts.Format, formatText), items, renderProvidersText)
		},
	}
}

func (a *cliApp) newModelsCommand(opts *rootOptions) *cobra.Command {
	var provider string
	var operation string
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List configured models",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig(opts.ConfigDir)
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			items := a.runtime(cfg, ".").ListModels(provider, "", operation)
			return a.render(cmd.OutOrStdout(), defaultFormat(opts.Format, formatText), items, renderModelsText)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider")
	cmd.Flags().StringVar(&operation, "operation", "", "Filter by operation")
	return cmd
}

func (a *cliApp) newModelCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "model <model>",
		Short: "Show model details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig(opts.ConfigDir)
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			model, ok := config.LookupModel(cfg, args[0])
			if !ok {
				return &exitError{Code: ExitConfig, Err: fmt.Errorf("unknown model %q", args[0])}
			}
			items := a.runtime(cfg, ".").ListModels("", args[0], "")
			response := map[string]any{
				"provider": model.Provider,
				"name":     model.Name,
				"endpoint": model.Endpoint,
				"detail":   items,
			}
			return a.render(cmd.OutOrStdout(), defaultFormat(opts.Format, formatText), response, renderModelText)
		},
	}
}

func (a *cliApp) newGenerateCommand(opts *rootOptions) *cobra.Command {
	var flags runFlags
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate an image",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig(opts.ConfigDir)
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			arguments, err := flags.generateArgs()
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			result, err := a.runtime(cfg, flags.outputDir()).Execute(context.Background(), imagine.ToolImageGenerate, arguments)
			if err != nil {
				return &exitError{Code: ExitExecution, Err: err}
			}
			return a.render(cmd.OutOrStdout(), defaultFormat(opts.Format, formatText), result, renderRunText)
		},
	}
	flags.bindCommon(cmd)
	cmd.Flags().StringVar(&flags.model, "model", "", "Model name")
	cmd.Flags().StringVar(&flags.prompt, "prompt", "", "Prompt")
	cmd.Flags().StringVar(&flags.responseFormat, "response-format", "", "Response format")
	cmd.Flags().StringVar(&flags.outputName, "output-name", "", "Output file name")
	return cmd
}

func (a *cliApp) newEditCommand(opts *rootOptions) *cobra.Command {
	var flags runFlags
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit an image",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig(opts.ConfigDir)
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			arguments, err := flags.editArgs()
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			result, err := a.runtime(cfg, flags.outputDir()).Execute(context.Background(), imagine.ToolImageEdit, arguments)
			if err != nil {
				return &exitError{Code: ExitExecution, Err: err}
			}
			return a.render(cmd.OutOrStdout(), defaultFormat(opts.Format, formatText), result, renderRunText)
		},
	}
	flags.bindCommon(cmd)
	cmd.Flags().StringVar(&flags.model, "model", "", "Model name")
	cmd.Flags().StringVar(&flags.prompt, "prompt", "", "Prompt")
	cmd.Flags().Var((*stringListValue)(&flags.images), "image", "Input image path or URL")
	cmd.Flags().StringVar(&flags.outputName, "output-name", "", "Output file name")
	return cmd
}

func (a *cliApp) newImportCommand(opts *rootOptions) *cobra.Command {
	var outputDir string
	var items []string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import external images",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig(opts.ConfigDir)
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			if len(items) == 0 {
				return &exitError{Code: ExitConfig, Err: fmt.Errorf("at least one --item is required")}
			}
			parsed := make([]any, 0, len(items))
			for _, raw := range items {
				var node map[string]any
				if err := json.Unmarshal([]byte(raw), &node); err != nil {
					return &exitError{Code: ExitConfig, Err: fmt.Errorf("invalid --item %q: %v", raw, err)}
				}
				parsed = append(parsed, node)
			}
			result, err := a.runtime(cfg, firstNonEmpty(outputDir, ".")).Execute(context.Background(), imagine.ToolImageImport, map[string]any{"items": parsed})
			if err != nil {
				return &exitError{Code: ExitExecution, Err: err}
			}
			return a.render(cmd.OutOrStdout(), defaultFormat(opts.Format, formatText), result, renderRunText)
		},
	}
	cmd.Flags().Var((*stringListValue)(&items), "item", "Import item as JSON object")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory")
	return cmd
}

func (a *cliApp) newRunCommand(opts *rootOptions) *cobra.Command {
	var flags runFlags
	cmd := &cobra.Command{
		Use:   "run <tool>",
		Short: "Run a tool directly",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig(opts.ConfigDir)
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			arguments, err := flags.genericArgs()
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			result, err := a.runtime(cfg, flags.outputDir()).Execute(context.Background(), args[0], arguments)
			if err != nil {
				return &exitError{Code: ExitExecution, Err: err}
			}
			return a.render(cmd.OutOrStdout(), defaultFormat(opts.Format, formatText), result, renderRunText)
		},
	}
	flags.bindCommon(cmd)
	return cmd
}

func (a *cliApp) newInspectCommand(opts *rootOptions) *cobra.Command {
	var flags runFlags
	cmd := &cobra.Command{
		Use:   "inspect <tool>",
		Short: "Inspect a tool call without executing it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig(opts.ConfigDir)
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			arguments, err := flags.genericArgs()
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			inspection, err := a.runtime(cfg, flags.outputDir()).Inspect(args[0], arguments)
			if err != nil {
				return &exitError{Code: ExitExecution, Err: err}
			}
			return a.render(cmd.OutOrStdout(), defaultFormat(opts.Format, formatJSON), inspection, renderInspectText)
		},
	}
	flags.bindCommon(cmd)
	return cmd
}

func (a *cliApp) newConfigCommand(opts *rootOptions) *cobra.Command {
	root := &cobra.Command{Use: "config", Short: "Manage configuration"}
	root.AddCommand(
		&cobra.Command{
			Use:   "validate",
			Short: "Validate provider configuration",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := a.loadConfig(opts.ConfigDir)
				if err != nil {
					return &exitError{Code: ExitConfig, Err: err}
				}
				response := map[string]any{
					"config_dir": opts.ConfigDir,
					"providers":  len(cfg.Providers),
					"models":     len(cfg.Models),
					"valid":      true,
				}
				return a.render(cmd.OutOrStdout(), defaultFormat(opts.Format, formatText), response, renderValidateText)
			},
		},
		a.newImportYAMLCommand(opts),
	)
	return root
}

func (a *cliApp) newImportYAMLCommand(opts *rootOptions) *cobra.Command {
	var fromPath string
	var toDir string
	cmd := &cobra.Command{
		Use:   "import-yaml",
		Short: "Convert old YAML provider configs into TOML + schema JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(fromPath) == "" || strings.TrimSpace(toDir) == "" {
				return &exitError{Code: ExitConfig, Err: fmt.Errorf("--from and --to are required")}
			}
			written, err := config.ImportYAML(fromPath, toDir)
			if err != nil {
				return &exitError{Code: ExitExecution, Err: err}
			}
			response := map[string]any{"written": written}
			return a.render(cmd.OutOrStdout(), defaultFormat(opts.Format, formatText), response, renderImportText)
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "Source YAML file or directory")
	cmd.Flags().StringVar(&toDir, "to", "", "Target config directory")
	return cmd
}

	func (a *cliApp) newVerifyCommand(opts *rootOptions) *cobra.Command {
	var outputDir string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify configured models with real requests",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig(opts.ConfigDir)
			if err != nil {
				return &exitError{Code: ExitConfig, Err: err}
			}
			summary, err := verify.Run(context.Background(), verify.Options{
				Config:    cfg,
				ConfigDir: opts.ConfigDir,
				OutputDir: firstNonEmpty(outputDir, "."),
			})
			if err != nil {
				return &exitError{Code: ExitExecution, Err: err}
			}
			return a.render(cmd.OutOrStdout(), defaultFormat(opts.Format, formatText), summary, renderVerifyText)
		},
	}
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory")
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Summary())
			return err
		},
	}
}

type runFlags struct {
	argsJSON       string
	argsFile       string
	argValues      jsonArgMap
	model          string
	prompt         string
	responseFormat string
	outputName     string
	outputDirValue string
	images         []string
}

func (f *runFlags) bindCommon(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.argsJSON, "args", "", "Raw JSON arguments object")
	cmd.Flags().StringVar(&f.argsFile, "args-file", "", "Path to JSON arguments file")
	cmd.Flags().Var(&f.argValues, "arg", "Single argument override in key=json form")
	cmd.Flags().StringVar(&f.outputDirValue, "output-dir", "", "Output directory")
}

func (f *runFlags) outputDir() string {
	return firstNonEmpty(f.outputDirValue, ".")
}

func (f *runFlags) genericArgs() (map[string]any, error) {
	result := map[string]any{}
	if strings.TrimSpace(f.argsFile) != "" {
		raw, err := os.ReadFile(f.argsFile)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("decode args-file: %v", err)
		}
	}
	if strings.TrimSpace(f.argsJSON) != "" {
		var node map[string]any
		if err := json.Unmarshal([]byte(f.argsJSON), &node); err != nil {
			return nil, fmt.Errorf("decode --args: %v", err)
		}
		for key, value := range node {
			result[key] = value
		}
	}
	for key, value := range f.argValues {
		result[key] = value
	}
	return result, nil
}

func (f *runFlags) generateArgs() (map[string]any, error) {
	result, err := f.genericArgs()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(f.model) != "" {
		result["model"] = f.model
	}
	if strings.TrimSpace(f.prompt) != "" {
		result["prompt"] = f.prompt
	}
	if strings.TrimSpace(f.responseFormat) != "" {
		result["responseFormat"] = f.responseFormat
	}
	if strings.TrimSpace(f.outputName) != "" {
		result["outputName"] = f.outputName
	}
	return result, nil
}

func (f *runFlags) editArgs() (map[string]any, error) {
	result, err := f.generateArgs()
	if err != nil {
		return nil, err
	}
	if len(f.images) > 0 {
		values := make([]any, 0, len(f.images))
		for _, image := range f.images {
			values = append(values, image)
		}
		result["images"] = values
	}
	return result, nil
}

func defaultFormat(current outputFormat, fallback outputFormat) outputFormat {
	if current == "" {
		return fallback
	}
	return current
}

func (a *cliApp) render(w io.Writer, format outputFormat, value any, renderText func(io.Writer, any) error) error {
	switch format {
	case formatJSON:
		return writeJSON(w, value)
	default:
		return renderText(w, value)
	}
}

func renderProvidersText(w io.Writer, value any) error {
	items := value.([]map[string]any)
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROVIDER\tMODELS\tPATH")
	for _, item := range items {
		_, _ = fmt.Fprintf(tw, "%s\t%v\t%s\n", item["name"], item["models"], item["path"])
	}
	return tw.Flush()
}

func renderModelsText(w io.Writer, value any) error {
	items := value.([]imagine.DiscoveryModel)
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROVIDER\tMODEL\tOPERATIONS")
	for _, item := range items {
		ops := make([]string, 0, len(item.Operations))
		for _, op := range item.Operations {
			ops = append(ops, op.Name)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", item.Provider, item.Name, strings.Join(ops, ","))
	}
	return tw.Flush()
}

func renderModelText(w io.Writer, value any) error {
	response := value.(map[string]any)
	_, err := fmt.Fprintf(w, "model: %s\nprovider: %s\nbase_url: %s\n", response["name"], response["provider"], response["endpoint"].(config.ModelEndpointConfig).BaseURL)
	return err
}

func renderRunText(w io.Writer, value any) error {
	node := value.(map[string]any)
	if asset, ok := node["asset"].(map[string]any); ok {
		_, err := fmt.Fprintln(w, asset["relativePath"])
		return err
	}
	if assets, ok := node["assets"].([]map[string]any); ok {
		for _, item := range assets {
			if _, err := fmt.Fprintln(w, item["relativePath"]); err != nil {
				return err
			}
		}
		return nil
	}
	return writeJSON(w, node)
}

func renderInspectText(w io.Writer, value any) error {
	return writeJSON(w, value)
}

func renderValidateText(w io.Writer, value any) error {
	node := value.(map[string]any)
	_, err := fmt.Fprintf(w, "config is valid: %v providers, %v models\n", node["providers"], node["models"])
	return err
}

func renderImportText(w io.Writer, value any) error {
	node := value.(map[string]any)
	written, _ := node["written"].([]string)
	sort.Strings(written)
	for _, path := range written {
		if _, err := fmt.Fprintln(w, path); err != nil {
			return err
		}
	}
	return nil
}

func renderVerifyText(w io.Writer, value any) error {
	summary := value.(verify.Summary)
	_, err := fmt.Fprintf(w, "summary: %s\n", summary.SummaryPath)
	return err
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
