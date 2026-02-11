package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"minitower/internal/towerfile"
)

func run(args []string) error {
	if len(args) == 0 {
		printRootUsage(os.Stderr)
		return &exitError{Code: 1}
	}

	switch args[0] {
	case "-h", "--help", "help":
		printRootUsage(os.Stdout)
		return nil
	case "deploy":
		return cmdDeploy(args[1:])
	case "login":
		return cmdLogin(args[1:])
	case "config":
		return cmdConfig(args[1:])
	case "me":
		return cmdMe(args[1:])
	case "apps":
		return cmdApps(args[1:])
	case "versions":
		return cmdVersions(args[1:])
	case "runs":
		return cmdRuns(args[1:])
	case "tokens":
		return cmdTokens(args[1:])
	case "runners":
		return cmdRunners(args[1:])
	default:
		printRootUsage(os.Stderr)
		return &exitError{Code: 1, Message: fmt.Sprintf("unknown command: %s", args[0])}
	}
}

func printRootUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: minitower-cli <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  login                             login with team credentials")
	fmt.Fprintln(w, "  config <set|get|list|use>         manage local profiles")
	fmt.Fprintln(w, "  me                                show current identity")
	fmt.Fprintln(w, "  apps <list|get|create>            manage apps")
	fmt.Fprintln(w, "  versions <list|get|upload>        manage versions")
	fmt.Fprintln(w, "  runs <create|list|get|cancel|retry|watch|logs>")
	fmt.Fprintln(w, "  tokens <create|list|revoke>       manage tokens (list/revoke pending API)")
	fmt.Fprintln(w, "  runners list                      list runners (admin)")
	fmt.Fprintln(w, "  deploy                            deploy from Towerfile")
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func ensureNoExtraArgs(fs *flag.FlagSet) error {
	if fs.NArg() > 0 {
		return &exitError{Code: 1, Message: fmt.Sprintf("unexpected arguments: %s", strings.Join(fs.Args(), " "))}
	}
	return nil
}

func resolveCommandConnection(profileName, server, token string, requireToken bool) (*apiClient, *resolvedConnection, error) {
	conn, err := resolveConnection(profileName, server, token, requireToken)
	if err != nil {
		return nil, nil, &exitError{Code: 1, Message: err.Error()}
	}
	return newAPIClient(conn.Server, conn.Token), conn, nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printAppTable(apps []appResponse) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "APP_ID\tSLUG\tDISABLED\tDESCRIPTION\tUPDATED_AT")
	for _, app := range apps {
		desc := ""
		if app.Description != nil {
			desc = *app.Description
		}
		fmt.Fprintf(tw, "%d\t%s\t%t\t%s\t%s\n", app.AppID, app.Slug, app.Disabled, desc, app.UpdatedAt)
	}
	_ = tw.Flush()
}

func printVersionTable(versions []versionResponse) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "VERSION_NO\tVERSION_ID\tENTRYPOINT\tSHA256\tCREATED_AT")
	for _, v := range versions {
		sha := v.ArtifactSHA256
		if len(sha) > 12 {
			sha = sha[:12]
		}
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%s\n", v.VersionNo, v.VersionID, v.Entrypoint, sha, v.CreatedAt)
	}
	_ = tw.Flush()
}

func printRunTable(runs []runResponse) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RUN_ID\tRUN_NO\tAPP\tSTATUS\tVERSION\tQUEUED_AT")
	for _, r := range runs {
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%d\t%s\n", r.RunID, r.RunNo, r.AppSlug, r.Status, r.VersionNo, r.QueuedAt)
	}
	_ = tw.Flush()
}

func printRunnerTable(runners []adminRunnerResponse) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RUNNER_ID\tNAME\tENVIRONMENT\tSTATUS\tLAST_SEEN_AT")
	for _, r := range runners {
		lastSeen := ""
		if r.LastSeenAt != nil {
			lastSeen = *r.LastSeenAt
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", r.RunnerID, r.Name, r.Environment, r.Status, lastSeen)
	}
	_ = tw.Flush()
}

func printLogs(logs []runLogEntry) {
	for _, l := range logs {
		fmt.Printf("[%d] %s %s\n", l.Seq, strings.ToUpper(l.Stream), l.Line)
	}
}

func apiStatusExitCode(status int) int {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return 10
	case http.StatusNotFound:
		return 11
	case http.StatusConflict:
		return 12
	case http.StatusGone:
		return 13
	default:
		return 1
	}
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	var ae *apiError
	if errors.As(err, &ae) {
		return &exitError{Code: apiStatusExitCode(ae.Status), Message: ae.Message}
	}
	return err
}

func cmdLogin(args []string) error {
	fs := newFlagSet("login")
	server := fs.String("server", "", "server URL")
	team := fs.String("team", "", "team slug")
	password := fs.String("password", "", "team password")
	profileName := fs.String("profile", "", "profile name")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}

	cfg, err := loadProfileConfig()
	if err != nil {
		return err
	}

	name := normalizeProfileName(*profileName)
	if name == defaultProfile && cfg.CurrentProfile != "" && *profileName == "" {
		name = normalizeProfileName(cfg.CurrentProfile)
	}

	existing := cfg.Profiles[name]
	if existing == nil {
		existing = &profile{}
	}

	resolvedServer := strings.TrimSpace(*server)
	if resolvedServer == "" {
		resolvedServer = strings.TrimSpace(os.Getenv(envServerURL))
	}
	if resolvedServer == "" {
		resolvedServer = strings.TrimSpace(existing.Server)
	}
	if resolvedServer == "" {
		return &exitError{Code: 1, Message: fmt.Sprintf("server URL is required (--server or %s)", envServerURL)}
	}

	resolvedTeam := strings.TrimSpace(*team)
	if resolvedTeam == "" {
		resolvedTeam = strings.TrimSpace(existing.Team)
	}
	if resolvedTeam == "" {
		return &exitError{Code: 1, Message: "team slug is required (--team)"}
	}

	resolvedPassword := strings.TrimSpace(*password)
	if resolvedPassword == "" {
		fmt.Fprint(os.Stderr, "Password: ")
		line, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
		fmt.Fprintln(os.Stderr)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return &exitError{Code: 1, Message: fmt.Sprintf("read password: %v", readErr)}
		}
		resolvedPassword = strings.TrimSpace(line)
	}
	if resolvedPassword == "" {
		return &exitError{Code: 1, Message: "password is required"}
	}

	client := newAPIClient(resolvedServer, "")
	var resp loginResponse
	err = client.doJSON(context.Background(), http.MethodPost, "/api/v1/teams/login", map[string]string{
		"slug":     resolvedTeam,
		"password": resolvedPassword,
	}, &resp)
	if err != nil {
		return mapError(err)
	}

	existing.Server = resolvedServer
	existing.Token = resp.Token
	existing.Team = resolvedTeam
	cfg.Profiles[name] = existing
	cfg.CurrentProfile = name
	if err := saveProfileConfig(cfg); err != nil {
		return err
	}

	if *jsonOut {
		return printJSON(map[string]any{
			"profile": name,
			"login":   resp,
		})
	}

	fmt.Printf("Logged in as team %q (role: %s)\n", resolvedTeam, resp.Role)
	fmt.Printf("Profile %q updated\n", name)
	return nil
}

func cmdConfig(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: minitower-cli config <set|get|list|use> ...")
		return &exitError{Code: 1}
	}

	switch args[0] {
	case "set":
		return cmdConfigSet(args[1:])
	case "get":
		return cmdConfigGet(args[1:])
	case "list":
		return cmdConfigList(args[1:])
	case "use":
		return cmdConfigUse(args[1:])
	default:
		return &exitError{Code: 1, Message: fmt.Sprintf("unknown config subcommand: %s", args[0])}
	}
}

func cmdConfigSet(args []string) error {
	fs := newFlagSet("config set")
	profileName := fs.String("profile", "", "profile name")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	team := fs.String("team", "", "default team slug")
	app := fs.String("app", "", "default app slug")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}

	cfg, err := loadProfileConfig()
	if err != nil {
		return err
	}

	name := normalizeProfileName(*profileName)
	if *profileName == "" && cfg.CurrentProfile != "" {
		name = normalizeProfileName(cfg.CurrentProfile)
	}

	p := cfg.Profiles[name]
	if p == nil {
		p = &profile{}
	}

	updated := false
	if strings.TrimSpace(*server) != "" {
		p.Server = strings.TrimSpace(*server)
		updated = true
	}
	if strings.TrimSpace(*token) != "" {
		p.Token = strings.TrimSpace(*token)
		updated = true
	}
	if strings.TrimSpace(*team) != "" {
		p.Team = strings.TrimSpace(*team)
		updated = true
	}
	if strings.TrimSpace(*app) != "" {
		p.App = strings.TrimSpace(*app)
		updated = true
	}

	if !updated {
		return &exitError{Code: 1, Message: "no changes provided (set one of: --server --token --team --app)"}
	}

	cfg.Profiles[name] = p
	if cfg.CurrentProfile == "" {
		cfg.CurrentProfile = name
	}
	if err := saveProfileConfig(cfg); err != nil {
		return err
	}

	if *jsonOut {
		return printJSON(map[string]any{"profile": name, "config": p})
	}
	fmt.Printf("Profile %q updated\n", name)
	return nil
}

func cmdConfigGet(args []string) error {
	fs := newFlagSet("config get")
	profileName := fs.String("profile", "", "profile name")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}

	cfg, err := loadProfileConfig()
	if err != nil {
		return err
	}
	name, p, err := pickProfile(cfg, *profileName)
	if err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if p == nil {
		return &exitError{Code: 1, Message: "no profile configured"}
	}

	if *jsonOut {
		return printJSON(map[string]any{
			"profile":         name,
			"is_current":      name == normalizeProfileName(cfg.CurrentProfile),
			"current_profile": normalizeProfileName(cfg.CurrentProfile),
			"config":          p,
		})
	}

	fmt.Printf("Profile: %s\n", name)
	if name == normalizeProfileName(cfg.CurrentProfile) {
		fmt.Fprintln(os.Stdout, "Current: true")
	}
	fmt.Printf("Server: %s\n", p.Server)
	fmt.Printf("Team: %s\n", p.Team)
	fmt.Printf("Default App: %s\n", p.App)
	if p.Token != "" {
		fmt.Fprintln(os.Stdout, "Token: set")
	} else {
		fmt.Fprintln(os.Stdout, "Token: not set")
	}
	return nil
}

func cmdConfigList(args []string) error {
	fs := newFlagSet("config list")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}

	cfg, err := loadProfileConfig()
	if err != nil {
		return err
	}

	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	if *jsonOut {
		profiles := make([]map[string]any, 0, len(names))
		for _, name := range names {
			profiles = append(profiles, map[string]any{
				"name":       name,
				"is_current": name == normalizeProfileName(cfg.CurrentProfile),
				"config":     cfg.Profiles[name],
			})
		}
		return printJSON(map[string]any{
			"current_profile": normalizeProfileName(cfg.CurrentProfile),
			"profiles":        profiles,
		})
	}

	if len(names) == 0 {
		fmt.Fprintln(os.Stdout, "No profiles configured.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CURRENT\tPROFILE\tSERVER\tTEAM\tAPP\tTOKEN")
	for _, name := range names {
		p := cfg.Profiles[name]
		current := ""
		if name == normalizeProfileName(cfg.CurrentProfile) {
			current = "*"
		}
		token := ""
		if p.Token != "" {
			token = "set"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", current, name, p.Server, p.Team, p.App, token)
	}
	_ = tw.Flush()
	return nil
}

func cmdConfigUse(args []string) error {
	fs := newFlagSet("config use")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if fs.NArg() != 1 {
		return &exitError{Code: 1, Message: "usage: minitower-cli config use <profile>"}
	}
	name := normalizeProfileName(fs.Arg(0))

	cfg, err := loadProfileConfig()
	if err != nil {
		return err
	}
	if cfg.Profiles[name] == nil {
		return &exitError{Code: 1, Message: fmt.Sprintf("profile %q not found", name)}
	}
	cfg.CurrentProfile = name
	if err := saveProfileConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("Current profile set to %q\n", name)
	return nil
}

func cmdMe(args []string) error {
	fs := newFlagSet("me")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}

	var resp meResponse
	if err := client.doJSON(context.Background(), http.MethodGet, "/api/v1/me", nil, &resp); err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	fmt.Printf("Team: %s (id=%d)\n", resp.TeamSlug, resp.TeamID)
	fmt.Printf("Token ID: %d\n", resp.TokenID)
	fmt.Printf("Role: %s\n", resp.Role)
	return nil
}

func cmdApps(args []string) error {
	if len(args) == 0 {
		return &exitError{Code: 1, Message: "usage: minitower-cli apps <list|get|create> ..."}
	}
	var err error
	switch args[0] {
	case "list":
		err = cmdAppsList(args[1:])
	case "get":
		err = cmdAppsGet(args[1:])
	case "create":
		err = cmdAppsCreate(args[1:])
	default:
		err = &exitError{Code: 1, Message: fmt.Sprintf("unknown apps subcommand: %s", args[0])}
	}
	return err
}

func cmdAppsList(args []string) error {
	fs := newFlagSet("apps list")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}

	var resp listAppsResponse
	if err := client.doJSON(context.Background(), http.MethodGet, "/api/v1/apps", nil, &resp); err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	printAppTable(resp.Apps)
	return nil
}

func cmdAppsGet(args []string) error {
	fs := newFlagSet("apps get")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if fs.NArg() != 1 {
		return &exitError{Code: 1, Message: "usage: minitower-cli apps get <app>"}
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}
	app := strings.TrimSpace(fs.Arg(0))

	var resp appResponse
	path := "/api/v1/apps/" + url.PathEscape(app)
	if err := client.doJSON(context.Background(), http.MethodGet, path, nil, &resp); err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	printAppTable([]appResponse{resp})
	return nil
}

func cmdAppsCreate(args []string) error {
	fs := newFlagSet("apps create")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	slug := fs.String("slug", "", "app slug")
	description := fs.String("description", "", "description")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}

	if strings.TrimSpace(*slug) == "" {
		if fs.NArg() != 1 {
			return &exitError{Code: 1, Message: "usage: minitower-cli apps create <slug> [--description TEXT]"}
		}
		*slug = strings.TrimSpace(fs.Arg(0))
	} else if fs.NArg() > 0 {
		return &exitError{Code: 1, Message: fmt.Sprintf("unexpected arguments: %s", strings.Join(fs.Args(), " "))}
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}

	var desc *string
	if strings.TrimSpace(*description) != "" {
		trimmed := strings.TrimSpace(*description)
		desc = &trimmed
	}

	var resp appResponse
	err = client.doJSON(context.Background(), http.MethodPost, "/api/v1/apps", map[string]any{
		"slug":        strings.TrimSpace(*slug),
		"description": desc,
	}, &resp)
	if err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	fmt.Printf("App %q created (id=%d)\n", resp.Slug, resp.AppID)
	return nil
}

func cmdVersions(args []string) error {
	if len(args) == 0 {
		return &exitError{Code: 1, Message: "usage: minitower-cli versions <list|get|upload> ..."}
	}
	switch args[0] {
	case "list":
		return cmdVersionsList(args[1:])
	case "get":
		return cmdVersionsGet(args[1:])
	case "upload":
		return cmdVersionsUpload(args[1:])
	default:
		return &exitError{Code: 1, Message: fmt.Sprintf("unknown versions subcommand: %s", args[0])}
	}
}

func defaultAppOrFlag(flagValue, fallback string) (string, error) {
	app := strings.TrimSpace(flagValue)
	if app == "" {
		app = strings.TrimSpace(fallback)
	}
	if app == "" {
		return "", &exitError{Code: 1, Message: "app is required (--app or profile default app)"}
	}
	return app, nil
}

func cmdVersionsList(args []string) error {
	fs := newFlagSet("versions list")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	appFlag := fs.String("app", "", "app slug")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}

	client, conn, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}
	app, err := defaultAppOrFlag(*appFlag, conn.DefaultApp)
	if err != nil {
		return err
	}

	var resp listVersionsResponse
	path := "/api/v1/apps/" + url.PathEscape(app) + "/versions"
	if err := client.doJSON(context.Background(), http.MethodGet, path, nil, &resp); err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	printVersionTable(resp.Versions)
	return nil
}

func cmdVersionsGet(args []string) error {
	fs := newFlagSet("versions get")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	appFlag := fs.String("app", "", "app slug")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if fs.NArg() != 1 {
		return &exitError{Code: 1, Message: "usage: minitower-cli versions get <version-no> --app <app>"}
	}

	versionNo, err := strconv.ParseInt(strings.TrimSpace(fs.Arg(0)), 10, 64)
	if err != nil || versionNo <= 0 {
		return &exitError{Code: 1, Message: "version number must be a positive integer"}
	}

	client, conn, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}
	app, err := defaultAppOrFlag(*appFlag, conn.DefaultApp)
	if err != nil {
		return err
	}

	var resp listVersionsResponse
	path := "/api/v1/apps/" + url.PathEscape(app) + "/versions"
	if err := client.doJSON(context.Background(), http.MethodGet, path, nil, &resp); err != nil {
		return mapError(err)
	}

	for _, v := range resp.Versions {
		if v.VersionNo == versionNo {
			if *jsonOut {
				return printJSON(v)
			}
			printVersionTable([]versionResponse{v})
			return nil
		}
	}

	return &exitError{Code: 11, Message: fmt.Sprintf("version %d not found", versionNo)}
}

func cmdVersionsUpload(args []string) error {
	fs := newFlagSet("versions upload")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	appFlag := fs.String("app", "", "app slug")
	filePath := fs.String("file", "", "artifact path (.tar.gz)")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}
	if strings.TrimSpace(*filePath) == "" {
		return &exitError{Code: 1, Message: "--file is required"}
	}

	client, conn, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}
	app, err := defaultAppOrFlag(*appFlag, conn.DefaultApp)
	if err != nil {
		return err
	}

	artifactData, err := os.ReadFile(*filePath)
	if err != nil {
		return &exitError{Code: 1, Message: fmt.Sprintf("read artifact: %v", err)}
	}

	var resp versionResponse
	uploadPath := "/api/v1/apps/" + url.PathEscape(app) + "/versions"
	err = client.doMultipartFile(context.Background(), uploadPath, "artifact", filepath.Base(*filePath), artifactData, &resp)
	if err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	fmt.Printf("Uploaded version %d for app %q (sha256:%s)\n", resp.VersionNo, app, shortenSHA(resp.ArtifactSHA256))
	return nil
}

func cmdDeploy(args []string) error {
	fs := newFlagSet("deploy")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	dir := fs.String("dir", ".", "project directory")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}

	result, err := deployFromDir(context.Background(), client, *dir)
	if err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(result)
	}
	fmt.Printf("Deploying app %q from %s\n", result.AppSlug, *dir)
	fmt.Printf("Artifact packaged (%d bytes, sha256:%s)\n", result.ArtifactBytes, shortenSHA(result.PackagedSHA))
	fmt.Printf("Version %d created (sha256:%s)\n", result.Version.VersionNo, shortenSHA(result.Version.ArtifactSHA256))
	return nil
}

type deployResult struct {
	AppSlug       string          `json:"app_slug"`
	ArtifactBytes int             `json:"artifact_bytes"`
	PackagedSHA   string          `json:"packaged_sha256"`
	Version       versionResponse `json:"version"`
}

func deployFromDir(ctx context.Context, client *apiClient, dir string) (*deployResult, error) {
	tfPath := filepath.Join(dir, "Towerfile")
	f, err := os.Open(tfPath)
	if err != nil {
		return nil, &exitError{Code: 1, Message: fmt.Sprintf("cannot open Towerfile: %v", err)}
	}
	defer f.Close()

	tf, err := towerfile.Parse(f)
	if err != nil {
		return nil, &exitError{Code: 1, Message: fmt.Sprintf("parsing Towerfile: %v", err)}
	}
	if err := towerfile.Validate(tf); err != nil {
		return nil, &exitError{Code: 1, Message: fmt.Sprintf("validating Towerfile: %v", err)}
	}

	artifact, sha256, err := towerfile.Package(dir, tf)
	if err != nil {
		return nil, &exitError{Code: 1, Message: fmt.Sprintf("packaging artifact: %v", err)}
	}

	artifactData, err := io.ReadAll(artifact)
	if err != nil {
		return nil, &exitError{Code: 1, Message: fmt.Sprintf("reading artifact: %v", err)}
	}

	if err := ensureApp(ctx, client, tf.App.Name); err != nil {
		return nil, err
	}

	var version versionResponse
	uploadPath := "/api/v1/apps/" + url.PathEscape(tf.App.Name) + "/versions"
	err = client.doMultipartFile(ctx, uploadPath, "artifact", "artifact.tar.gz", artifactData, &version)
	if err != nil {
		return nil, err
	}

	return &deployResult{
		AppSlug:       tf.App.Name,
		ArtifactBytes: len(artifactData),
		PackagedSHA:   sha256,
		Version:       version,
	}, nil
}

func ensureApp(ctx context.Context, client *apiClient, slug string) error {
	var existing appResponse
	getPath := "/api/v1/apps/" + url.PathEscape(slug)
	err := client.doJSON(ctx, http.MethodGet, getPath, nil, &existing)
	if err == nil {
		return nil
	}

	var ae *apiError
	if !errors.As(err, &ae) || ae.Status != http.StatusNotFound {
		return err
	}

	var created appResponse
	if err := client.doJSON(ctx, http.MethodPost, "/api/v1/apps", map[string]string{"slug": slug}, &created); err != nil {
		return err
	}
	return nil
}

func cmdRuns(args []string) error {
	if len(args) == 0 {
		return &exitError{Code: 1, Message: "usage: minitower-cli runs <create|list|get|cancel|retry|watch|logs> ..."}
	}
	var err error
	switch args[0] {
	case "create":
		err = cmdRunsCreate(args[1:])
	case "list":
		err = cmdRunsList(args[1:])
	case "get":
		err = cmdRunsGet(args[1:])
	case "cancel":
		err = cmdRunsCancel(args[1:])
	case "retry":
		err = cmdRunsRetry(args[1:])
	case "watch":
		err = cmdRunsWatch(args[1:])
	case "logs":
		err = cmdRunsLogs(args[1:])
	default:
		err = &exitError{Code: 1, Message: fmt.Sprintf("unknown runs subcommand: %s", args[0])}
	}
	return err
}

func cmdRunsCreate(args []string) error {
	fs := newFlagSet("runs create")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	appFlag := fs.String("app", "", "app slug")
	inputJSON := fs.String("input", "", "input JSON object")
	version := fs.String("version", "", "version number")
	priority := fs.String("priority", "", "priority")
	maxRetries := fs.String("max-retries", "", "max retries")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}

	client, conn, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}
	app, err := defaultAppOrFlag(*appFlag, conn.DefaultApp)
	if err != nil {
		return err
	}

	payload := map[string]any{}
	if strings.TrimSpace(*inputJSON) != "" {
		var input map[string]any
		if err := json.Unmarshal([]byte(*inputJSON), &input); err != nil {
			return &exitError{Code: 1, Message: fmt.Sprintf("invalid --input JSON: %v", err)}
		}
		payload["input"] = input
	}
	if strings.TrimSpace(*version) != "" {
		val, err := strconv.ParseInt(strings.TrimSpace(*version), 10, 64)
		if err != nil || val <= 0 {
			return &exitError{Code: 1, Message: "--version must be a positive integer"}
		}
		payload["version_no"] = val
	}
	if strings.TrimSpace(*priority) != "" {
		val, err := strconv.Atoi(strings.TrimSpace(*priority))
		if err != nil {
			return &exitError{Code: 1, Message: "--priority must be an integer"}
		}
		payload["priority"] = val
	}
	if strings.TrimSpace(*maxRetries) != "" {
		val, err := strconv.Atoi(strings.TrimSpace(*maxRetries))
		if err != nil || val < 0 {
			return &exitError{Code: 1, Message: "--max-retries must be a non-negative integer"}
		}
		payload["max_retries"] = val
	}

	createPath := "/api/v1/apps/" + url.PathEscape(app) + "/runs"
	var resp runResponse
	err = client.doJSON(context.Background(), http.MethodPost, createPath, payload, &resp)
	if err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	fmt.Printf("Run #%d created (id=%d, status=%s)\n", resp.RunNo, resp.RunID, resp.Status)
	return nil
}

func cmdRunsList(args []string) error {
	fs := newFlagSet("runs list")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	app := fs.String("app", "", "app slug")
	status := fs.String("status", "", "status filter")
	limit := fs.Int("limit", 50, "max rows")
	offset := fs.Int("offset", 0, "offset")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}
	if *limit <= 0 || *limit > 100 {
		return &exitError{Code: 1, Message: "--limit must be between 1 and 100"}
	}
	if *offset < 0 {
		return &exitError{Code: 1, Message: "--offset must be >= 0"}
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}

	qPath, err := withQuery("/api/v1/runs", map[string]string{
		"app":    strings.TrimSpace(*app),
		"status": strings.TrimSpace(*status),
		"limit":  strconv.Itoa(*limit),
		"offset": strconv.Itoa(*offset),
	})
	if err != nil {
		return err
	}

	var resp listRunsResponse
	if err := client.doJSON(context.Background(), http.MethodGet, qPath, nil, &resp); err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	printRunTable(resp.Runs)
	return nil
}

func parseRunIDArg(arg string) (int64, error) {
	runID, err := strconv.ParseInt(strings.TrimSpace(arg), 10, 64)
	if err != nil || runID <= 0 {
		return 0, &exitError{Code: 1, Message: "run ID must be a positive integer"}
	}
	return runID, nil
}

func cmdRunsGet(args []string) error {
	fs := newFlagSet("runs get")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if fs.NArg() != 1 {
		return &exitError{Code: 1, Message: "usage: minitower-cli runs get <run-id>"}
	}
	runID, err := parseRunIDArg(fs.Arg(0))
	if err != nil {
		return err
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}
	var resp runResponse
	if err := client.doJSON(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/runs/%d", runID), nil, &resp); err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	printRunTable([]runResponse{resp})
	return nil
}

func cmdRunsCancel(args []string) error {
	fs := newFlagSet("runs cancel")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if fs.NArg() != 1 {
		return &exitError{Code: 1, Message: "usage: minitower-cli runs cancel <run-id>"}
	}
	runID, err := parseRunIDArg(fs.Arg(0))
	if err != nil {
		return err
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}

	var resp runResponse
	if err := client.doJSON(context.Background(), http.MethodPost, fmt.Sprintf("/api/v1/runs/%d/cancel", runID), nil, &resp); err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	fmt.Printf("Run %d status: %s\n", resp.RunID, resp.Status)
	return nil
}

func cmdRunsRetry(args []string) error {
	fs := newFlagSet("runs retry")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if fs.NArg() != 1 {
		return &exitError{Code: 1, Message: "usage: minitower-cli runs retry <run-id>"}
	}
	runID, err := parseRunIDArg(fs.Arg(0))
	if err != nil {
		return err
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}

	var current runResponse
	if err := client.doJSON(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/runs/%d", runID), nil, &current); err != nil {
		return mapError(err)
	}
	if strings.TrimSpace(current.AppSlug) == "" {
		return &exitError{Code: 1, Message: "run response missing app_slug; cannot retry"}
	}

	payload := map[string]any{
		"input":       current.Input,
		"version_no":  current.VersionNo,
		"priority":    current.Priority,
		"max_retries": current.MaxRetries,
	}
	createPath := "/api/v1/apps/" + url.PathEscape(current.AppSlug) + "/runs"
	var resp runResponse
	if err := client.doJSON(context.Background(), http.MethodPost, createPath, payload, &resp); err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	fmt.Printf("Retry created: run #%d (id=%d)\n", resp.RunNo, resp.RunID)
	return nil
}

func resolveWatchRunID(client *apiClient, runIDArg, appFlag string, defaultApp string) (int64, error) {
	if strings.TrimSpace(runIDArg) != "" {
		return parseRunIDArg(runIDArg)
	}

	app, err := defaultAppOrFlag(appFlag, defaultApp)
	if err != nil {
		return 0, err
	}
	listPath, err := withQuery("/api/v1/apps/"+url.PathEscape(app)+"/runs", map[string]string{
		"limit": "1",
	})
	if err != nil {
		return 0, err
	}
	var resp listRunsResponse
	if err := client.doJSON(context.Background(), http.MethodGet, listPath, nil, &resp); err != nil {
		return 0, mapError(err)
	}
	if len(resp.Runs) == 0 {
		return 0, &exitError{Code: 11, Message: fmt.Sprintf("no runs found for app %q", app)}
	}
	return resp.Runs[0].RunID, nil
}

func fetchRunLogs(client *apiClient, runID int64, afterSeq int64) ([]runLogEntry, error) {
	logPath, err := withQuery(fmt.Sprintf("/api/v1/runs/%d/logs", runID), map[string]string{
		"after_seq": strconv.FormatInt(afterSeq, 10),
	})
	if err != nil {
		return nil, err
	}
	var logsResp runLogsResponse
	if err := client.doJSON(context.Background(), http.MethodGet, logPath, nil, &logsResp); err != nil {
		return nil, err
	}
	return logsResp.Logs, nil
}

func cmdRunsWatch(args []string) error {
	fs := newFlagSet("runs watch")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	appFlag := fs.String("app", "", "app slug (required if run-id omitted)")
	statusOnly := fs.Bool("status-only", false, "watch status without logs")
	interval := fs.Duration("interval", 2*time.Second, "poll interval")
	jsonOut := fs.Bool("json", false, "print final run JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if *interval <= 0 {
		return &exitError{Code: 1, Message: "--interval must be > 0"}
	}
	if *jsonOut && !*statusOnly {
		return &exitError{Code: 1, Message: "--json is only supported with --status-only for runs watch"}
	}
	if fs.NArg() > 1 {
		return &exitError{Code: 1, Message: "usage: minitower-cli runs watch [run-id] [--app APP]"}
	}

	client, conn, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}

	runIDArg := ""
	if fs.NArg() == 1 {
		runIDArg = fs.Arg(0)
	}
	runID, err := resolveWatchRunID(client, runIDArg, *appFlag, conn.DefaultApp)
	if err != nil {
		return err
	}

	var afterSeq int64
	lastStatus := ""
	for {
		var run runResponse
		if err := client.doJSON(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/runs/%d", runID), nil, &run); err != nil {
			return mapError(err)
		}

		if run.Status != lastStatus {
			fmt.Printf("run %d status: %s\n", runID, run.Status)
			lastStatus = run.Status
		}

		if !*statusOnly {
			logs, err := fetchRunLogs(client, runID, afterSeq)
			if err != nil {
				return mapError(err)
			}
			if len(logs) > 0 {
				printLogs(logs)
				afterSeq = logs[len(logs)-1].Seq
			}
		}

		if isTerminalRunStatus(run.Status) {
			if !*statusOnly {
				logs, err := fetchRunLogs(client, runID, afterSeq)
				if err != nil {
					return mapError(err)
				}
				if len(logs) > 0 {
					printLogs(logs)
					afterSeq = logs[len(logs)-1].Seq
				}
			}
			if *jsonOut {
				if err := printJSON(run); err != nil {
					return err
				}
			}

			switch run.Status {
			case "completed":
				return nil
			case "cancelled":
				return &exitError{Code: 2}
			default:
				return &exitError{Code: 1}
			}
		}

		time.Sleep(*interval)
	}
}

func cmdRunsLogs(args []string) error {
	fs := newFlagSet("runs logs")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	follow := fs.Bool("follow", false, "follow logs")
	interval := fs.Duration("interval", 2*time.Second, "poll interval")
	after := fs.Int64("after-seq", 0, "start after sequence number")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if fs.NArg() != 1 {
		return &exitError{Code: 1, Message: "usage: minitower-cli runs logs <run-id> [--follow]"}
	}
	if *interval <= 0 {
		return &exitError{Code: 1, Message: "--interval must be > 0"}
	}
	if *jsonOut && *follow {
		return &exitError{Code: 1, Message: "--json is not supported with --follow"}
	}
	if *after < 0 {
		return &exitError{Code: 1, Message: "--after-seq must be non-negative"}
	}
	runID, err := parseRunIDArg(fs.Arg(0))
	if err != nil {
		return err
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}

	afterSeq := *after
	for {
		logs, err := fetchRunLogs(client, runID, afterSeq)
		if err != nil {
			return mapError(err)
		}
		if *jsonOut {
			return printJSON(runLogsResponse{Logs: logs})
		}
		if len(logs) > 0 {
			printLogs(logs)
			afterSeq = logs[len(logs)-1].Seq
		}
		if !*follow {
			return nil
		}

		var run runResponse
		if err := client.doJSON(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/runs/%d", runID), nil, &run); err != nil {
			return mapError(err)
		}
		if isTerminalRunStatus(run.Status) {
			logs, err := fetchRunLogs(client, runID, afterSeq)
			if err != nil {
				return mapError(err)
			}
			if len(logs) > 0 {
				printLogs(logs)
			}
			return nil
		}

		time.Sleep(*interval)
	}
}

func cmdTokens(args []string) error {
	if len(args) == 0 {
		return &exitError{Code: 1, Message: "usage: minitower-cli tokens <create|list|revoke> ..."}
	}
	switch args[0] {
	case "create":
		return cmdTokensCreate(args[1:])
	case "list", "revoke":
		return &exitError{Code: 1, Message: fmt.Sprintf("tokens %s is not available yet (API endpoint not implemented)", args[0])}
	default:
		return &exitError{Code: 1, Message: fmt.Sprintf("unknown tokens subcommand: %s", args[0])}
	}
}

func cmdTokensCreate(args []string) error {
	fs := newFlagSet("tokens create")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	name := fs.String("name", "", "token name")
	role := fs.String("role", "", "token role (admin|member)")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}

	if strings.TrimSpace(*role) != "" {
		r := strings.TrimSpace(*role)
		if r != "admin" && r != "member" {
			return &exitError{Code: 1, Message: "--role must be admin or member"}
		}
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}

	body := map[string]any{}
	if strings.TrimSpace(*name) != "" {
		body["name"] = strings.TrimSpace(*name)
	}
	if strings.TrimSpace(*role) != "" {
		body["role"] = strings.TrimSpace(*role)
	}

	var resp createTokenResponse
	if err := client.doJSON(context.Background(), http.MethodPost, "/api/v1/tokens", body, &resp); err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	fmt.Printf("Token created: id=%d role=%s\n", resp.TokenID, resp.Role)
	fmt.Printf("Token: %s\n", resp.Token)
	return nil
}

func cmdRunners(args []string) error {
	if len(args) == 0 || args[0] != "list" {
		return &exitError{Code: 1, Message: "usage: minitower-cli runners list"}
	}

	fs := newFlagSet("runners list")
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "API token")
	profileName := fs.String("profile", "", "profile name")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return &exitError{Code: 1, Message: err.Error()}
	}
	if err := ensureNoExtraArgs(fs); err != nil {
		return err
	}

	client, _, err := resolveCommandConnection(*profileName, *server, *token, true)
	if err != nil {
		return err
	}

	var resp listAdminRunnersResponse
	if err := client.doJSON(context.Background(), http.MethodGet, "/api/v1/admin/runners", nil, &resp); err != nil {
		return mapError(err)
	}

	if *jsonOut {
		return printJSON(resp)
	}
	printRunnerTable(resp.Runners)
	return nil
}

func shortenSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}
