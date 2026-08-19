package mcpagent

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/shrsv/AgentLaws/pkg/alaws"
)

//go:embed alaws_livi
var lawbookFS embed.FS

// lawbookOnce ensures the lawbook is loaded exactly once, regardless of how
// many Agent instances are created. The compiled Book is shared across all
// sessions.
var (
	lawbookOnce sync.Once
	lawbookErr  error
	lawbook     *alaws.Book
)

// lawbookPaths holds the pre-rendered prompt text for each analytics
// pipeline branch. Each branch selects a different subset of the lawbook's
// sections — the model only sees the laws relevant to the stage it's
// executing.
type lawbookPaths struct {
	// classify is call #0's system prompt: shape routing instructions.
	classify string
	// planHead is the static header for call #2 (before the live schema).
	planHead string
	// planTail is the static footer for call #2 (after the live schema).
	planTail string
	// finalizeHead is the static header for call #3 (before the live schema).
	finalizeHead string
	// finalizeTail is the static footer for call #3 (after the live schema).
	finalizeTail string
	// repair is the system prompt for a rejected-query retry.
	repair string
	// noData is the system prompt for a zero-row result.
	noData string
}

// loadLawbook compiles the embedded alaws_livi lawbook and returns the
// compiled Book. Called once via lawbookOnce.
func loadLawbook() (*alaws.Book, error) {
	// alaws.Load reads from the filesystem, not from fs.FS. We need to
	// extract the embedded tree to a temp directory first.
	tmpDir, err := extractLawbookFS()
	if err != nil {
		return nil, fmt.Errorf("extract lawbook: %w", err)
	}
	book, err := alaws.Load(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("compile lawbook: %w", err)
	}
	return book, nil
}

// extractLawbookFS writes the embedded alaws_livi/ tree to a temp directory
// and returns its path. The caller can defer os.RemoveAll if cleanup is
// desired, but since the process lifetime matches the lawbook's lifetime
// this is not strictly necessary.
func extractLawbookFS() (string, error) {
	tmpDir := filepath.Join(os.TempDir(), "livi-alaws-book")
	err := fs.WalkDir(lawbookFS, "alaws_livi", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(tmpDir, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, readErr := fs.ReadFile(lawbookFS, path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, 0644)
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(tmpDir, "alaws_livi"), nil
}

// ensureLawbook loads the lawbook once and stores it in the package-level
// lawbook variable. Returns the book or an error.
func ensureLawbook() (*alaws.Book, error) {
	lawbookOnce.Do(func() {
		lawbook, lawbookErr = loadLawbook()
		if lawbookErr == nil {
			diags := lawbook.Diagnostics()
			for _, d := range diags {
				log.Warn().Str("severity", d.Severity).Str("code", d.Code).Str("msg", d.Message).
					Msg("lawbook diagnostic")
			}
			log.Info().Int("sections", len(lawbook.Lawbook().Sections)).
				Msg("alaws lawbook compiled")
		}
	})
	return lawbook, lawbookErr
}

// renderBranch renders the laws from the given section ID prefixes into a
// single prompt-ready string. It collects all sections whose ID starts with
// any of the given prefixes (e.g. "livi.general" matches "livi.general",
// "livi.general.principles", "livi.general.data", etc.).
// specExampleMarker identifies the illustrative Vega-Lite specification
// carried by most chart-selection sections. Those examples exist for the
// finalizing stage, which draws the chart; the planning stage only needs
// to know what to group by, so it renders without them. Left in, they are
// roughly two thirds of the planning prompt and push the instructions
// that stage actually depends on far enough from the end that a small
// model stops following them.
const specExampleMarker = "The specification below is an example"

func renderBranch(book *alaws.Book, prefixes []string, vars map[string]string) (string, error) {
	return renderBranchFiltered(book, prefixes, vars, false)
}

func renderBranchFiltered(book *alaws.Book, prefixes []string, vars map[string]string, dropSpecs bool) (string, error) {
	// Rendered one prefix group at a time and joined in prefix order.
	// alaws.Book.Laws always returns laws in book order no matter what
	// order the Selector's SectionIDs are given in, so a single call
	// cannot put a chosen chapter last. Each branch needs its
	// output-format contract at the end of the prompt, where a small
	// model weights it most heavily, so the caller controls that by the
	// order it lists prefixes here.
	var parts []string
	for _, prefix := range prefixes {
		ids := collectSectionIDs(book, []string{prefix})
		if len(ids) == 0 {
			return "", fmt.Errorf("no sections matched prefix %q", prefix)
		}
		laws, err := book.Laws(alaws.Selector{SectionIDs: ids})
		if err != nil {
			return "", fmt.Errorf("select laws %q: %w", prefix, err)
		}
		if dropSpecs {
			kept := laws.Laws[:0]
			for _, l := range laws.Laws {
				if !strings.Contains(l.Text, specExampleMarker) {
					kept = append(kept, l)
				}
			}
			laws.Laws = kept
		}
		rendered, err := laws.Render(alaws.RenderOptions{
			Vars:      vars,
			OnMissing: alaws.MissingKeepPlaceholder,
		})
		if err != nil {
			return "", fmt.Errorf("render laws %q: %w", prefix, err)
		}
		parts = append(parts, strings.TrimSpace(rendered))
	}
	return strings.Join(parts, "\n\n"), nil
}

// collectSectionIDs returns all section IDs from the book matching the
// given prefixes, ordered by the PREFIX list rather than by book order.
//
// The order matters: a small model weights the end of a long prompt most
// heavily, and each branch's output-format contract is the one instruction
// it cannot afford to miss. Ordering by prefix lets a caller put the
// chapter carrying that contract last, without reordering the book itself
// (the book is ordered for a human reading it end to end). Within a
// prefix, book order is preserved.
func collectSectionIDs(book *alaws.Book, prefixes []string) []string {
	var ids []string
	seen := map[string]bool{}
	for _, prefix := range prefixes {
		for _, s := range book.Lawbook().Sections {
			if seen[s.ID] {
				continue
			}
			if s.ID == prefix || strings.HasPrefix(s.ID, prefix+".") {
				ids = append(ids, s.ID)
				seen[s.ID] = true
			}
		}
	}
	return ids
}

// buildLawbookPrompts renders the four analytics pipeline branches from the
// compiled lawbook. Called by WithAnalytics after the lawbook is loaded.
//
// The mapping from lawbook chapters to pipeline stages follows the
// introduction.md's "How a question is answered" section:
//
//	Classify:  General + Classification
//	Plan:      General + Planning + Chart Selection
//	Finalize:  General + Chart Selection + Finalizing
//	Degraded:  General + Degraded Paths
func buildLawbookPrompts(orgName, userName string, orgID int64) (*lawbookPaths, error) {
	book, err := ensureLawbook()
	if err != nil {
		return nil, err
	}

	vars := map[string]string{
		"org_id": fmt.Sprintf("%d", orgID),
	}

	classify, err := renderBranch(book, []string{"livi.general", "livi.classify"}, vars)
	if err != nil {
		return nil, fmt.Errorf("classify branch: %w", err)
	}

	planLaws, err := renderBranchFiltered(book, []string{"livi.general", "livi.charts", "livi.planning"}, vars, true)
	if err != nil {
		return nil, fmt.Errorf("plan branch: %w", err)
	}

	finalizeLaws, err := renderBranch(book, []string{"livi.general", "livi.charts", "livi.finalizing"}, vars)
	if err != nil {
		return nil, fmt.Errorf("finalize branch: %w", err)
	}

	repairLaws, err := renderBranch(book, []string{"livi.general", "livi.degraded"}, vars)
	if err != nil {
		return nil, fmt.Errorf("degraded branch: %w", err)
	}

	noDataLaws, err := renderBranch(book, []string{"livi.general", "livi.degraded"}, vars)
	if err != nil {
		return nil, fmt.Errorf("nodata branch: %w", err)
	}

	// Build the header that appears before the law text in each branch.
	// This includes the persona, org/user context, and the org_id filter
	// instruction — things that are session-specific, not lawbook content.
	header := buildPromptHeader(orgName, userName)
	orgFilter := orgIDFilterInstruction(orgID)

	return &lawbookPaths{
		classify: classify,
		// Plan head: header + org filter + the law text comes next, then
		// the live schema is spliced in, then planTail has the rest.
		// Actually: the law text IS the prompt now. The schema splice
		// happens in countQueryPrompt/finalizePrompt which still use the
		// head+tableText+tail pattern. So head = header + org filter +
		// plan laws (data rules + planning + chart selection), and the
		// live schema text is appended after. But wait — the old pattern
		// was: header + org filter + schema_intro + tableText + schema_examples
		// + plan_instructions. In the new system the lawbook replaces all
		// the static parts. The schema splice (tableText) goes between
		// the data rules and the rest.
		//
		// For now: head = header + org filter, tail = all plan laws.
		// The schema text is spliced between head and tail in
		// countQueryPrompt, same as before. This means the plan laws
		// (which include data rules) go AFTER the schema, which is
		// actually correct — the model sees the table listing first,
		// then the rules for how to use it.
		planHead:   header + "\n\n" + orgFilter,
		planTail:   "\n\n" + planLaws,
		finalizeHead: header + "\n\n" + orgFilter,
		finalizeTail: "\n\n" + finalizeLaws,
		// Repair/nodata: full prompt, no schema splice needed.
		repair: header + "\n\n" + orgFilter + "\n\n" + repairLaws,
		noData: header + "\n\n" + orgFilter + "\n\n" + noDataLaws,
	}, nil
}

// buildNonAnalyticsPrompts builds the action and chat branch prompts.
// These are NOT analytics-specific — they use the old agent_instructions.md
// and chat_only.md content, which govern tool-calling and conversation
// respectively. Kept as plain strings, not lawbook laws, because they
// don't benefit from the numbering/citation system.
func buildNonAnalyticsPrompts(orgName, userName string) (actionPrompt, chatPrompt, classifyPrompt string) {
	header := buildPromptHeader(orgName, userName)

	// Action branch: same as the pre-analytics tool-only prompt.
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n\n")
	if orgName != "" || userName != "" {
		b.WriteString("The user you are helping belongs to the following context. This is MANDATORY:\n")
		if userName != "" {
			b.WriteString(fmt.Sprintf("- User: %s\n", userName))
		}
		if orgName != "" {
			b.WriteString(fmt.Sprintf("- Organization: %s\n", orgName))
		}
		b.WriteString("\n")
		b.WriteString(orgPromptGuidance)
		b.WriteString("\n")
	}
	// agentInstructions is still loaded from the old embed; it will be
	// replaced once we decide whether to fold it into the lawbook.
	actionPrompt = b.String()

	// Chat branch: persona header + chat-only constraints.
	chatPrompt = header + "\n\n" + chatOnlyInstructions

	return
}
