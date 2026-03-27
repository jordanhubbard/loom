package loom

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/pkg/models"
)

func isSSHRepo(repo string) bool {
	repo = strings.TrimSpace(repo)
	return strings.HasPrefix(repo, "git@") || strings.HasPrefix(repo, "ssh://")
}
func roleFromPersonaName(personaName string) string {
	personaName = strings.TrimSpace(personaName)
	if strings.HasPrefix(personaName, "default/") {
		return strings.TrimPrefix(personaName, "default/")
	}
	if strings.HasPrefix(personaName, "projects/") {
		parts := strings.Split(personaName, "/")
		if len(parts) >= 3 {
			return parts[2]
		}
	}
	if strings.Contains(personaName, "/") {
		parts := strings.Split(personaName, "/")
		return parts[len(parts)-1]
	}
	return personaName
}

func capitalizeAcronyms(name string) string {
	// Only replace whole words (space-bounded or at start/end)
	words := strings.Split(name, " ")
	acronyms := map[string]string{
		"Ceo": "CEO",
		"Cfo": "CFO",
		"Qa":  "QA",
	}
	for i, word := range words {
		if replacement, ok := acronyms[word]; ok {
			words[i] = replacement
		}
	}
	return strings.Join(words, " ")
}
func normalizeGitAuthMethod(repo string, method models.GitAuthMethod) models.GitAuthMethod {
	if method != "" {
		return method
	}
	if repo == "" || repo == "." {
		return models.GitAuthNone
	}
	return models.GitAuthSSH
}
func normalizeGitStrategy(strategy models.GitStrategy) models.GitStrategy {
	if strategy != "" {
		return strategy
	}
	return models.GitStrategyDirect
}
func (a *Loom) allowedRoleSet() map[string]struct{} {
	roles := a.config.Agents.AllowedRoles
	if len(roles) == 0 {
		roles = rolesForProfile(a.config.Agents.CorpProfile)
	}
	if len(roles) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(strings.ToLower(role))
		if role == "" {
			continue
		}
		set[role] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}
func rolesForProfile(profile string) []string {
	profile = strings.TrimSpace(strings.ToLower(profile))
	switch profile {
	case "startup":
		return []string{"ceo", "engineering-manager", "web-designer"}
	case "solo":
		return []string{"ceo", "engineering-manager"}
	case "full", "enterprise", "":
		return nil
	default:
		return nil
	}
}

func normalizeRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	if strings.Contains(role, "/") {
		parts := strings.Split(role, "/")
		role = parts[len(parts)-1]
	}
	if idx := strings.Index(role, "("); idx != -1 {
		role = strings.TrimSpace(role[:idx])
	}
	role = strings.ReplaceAll(role, "_", "-")
	role = strings.ReplaceAll(role, " ", "-")
	return role
}

// tryAutoApproveCodeFix evaluates a code fix proposal for auto-approval.
// Low-risk fixes (single file, no security impact, small diff) are closed
// immediately. Higher-risk fixes stay open for agent review.
func (a *Loom) tryAutoApproveCodeFix(bead *models.Bead) {
	risk, reasons := assessFixRisk(bead.Description)
	log.Printf("[AutoApproval] Bead %s risk=%s reasons=%v", bead.ID, risk, reasons)

	if risk != "low" {
		log.Printf("[AutoApproval] Bead %s requires manual CEO review (risk=%s)", bead.ID, risk)
		return
	}

	// Wait briefly so the bead is fully persisted before we close it
	time.Sleep(2 * time.Second)

	reason := fmt.Sprintf("Auto-approved (risk=%s): %s", risk, strings.Join(reasons, "; "))
	if err := a.CloseBead(bead.ID, reason); err != nil {
		log.Printf("[AutoApproval] Failed to auto-approve bead %s: %v", bead.ID, err)
		return
	}
	log.Printf("[AutoApproval] Auto-approved code fix proposal %s", bead.ID)
}

// assessFixRisk evaluates the risk level of a code fix proposal.
// Returns risk level ("low", "medium", "high") and list of reasons.
func assessFixRisk(description string) (string, []string) {
	lower := strings.ToLower(description)
	var reasons []string

	// High-risk indicators
	highRiskPatterns := []string{
		"security", "authentication", "authorization", "password",
		"encryption", "token", "secret", "credential",
		"database migration", "schema change", "drop table",
		"delete all", "rm -rf", "force push",
	}
	for _, p := range highRiskPatterns {
		if strings.Contains(lower, p) {
			return "high", []string{"contains security/destructive keyword: " + p}
		}
	}

	// Medium-risk: multi-file changes, API changes, config changes
	mediumRiskPatterns := []string{
		"breaking change", "api change", "config change",
		"multiple files", "architecture", "refactor",
	}
	for _, p := range mediumRiskPatterns {
		if strings.Contains(lower, p) {
			reasons = append(reasons, "medium-risk pattern: "+p)
		}
	}
	if len(reasons) > 0 {
		return "medium", reasons
	}

	// Check risk assessment section in the proposal
	if strings.Contains(lower, "risk level: high") || strings.Contains(lower, "risk: high") {
		return "high", []string{"proposal self-assessed as high risk"}
	}
	if strings.Contains(lower, "risk level: medium") || strings.Contains(lower, "risk: medium") {
		return "medium", []string{"proposal self-assessed as medium risk"}
	}

	// Low-risk indicators
	if strings.Contains(lower, "risk level: low") || strings.Contains(lower, "risk: low") {
		reasons = append(reasons, "proposal self-assessed as low risk")
	}

	lowRiskPatterns := []string{
		"typo", "comment", "formatting", "whitespace",
		"variable rename", "css", "style", "cosmetic",
		"undefined variable", "missing import",
		"single file", "one file",
	}
	for _, p := range lowRiskPatterns {
		if strings.Contains(lower, p) {
			reasons = append(reasons, "low-risk pattern: "+p)
		}
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "no high/medium risk indicators detected")
	}

	return "low", reasons
}

func extractOriginalBugID(description string) string {
	// Look for patterns like "**Original Bug:** ac-001" or "Original Bug: bd-123"
	pats := []string{
		"**Original Bug:** ",
		"Original Bug: ",
		"**Original Bug:**",
	}

	for _, pattern := range pats {
		idx := strings.Index(description, pattern)
		if idx >= 0 {
			start := idx + len(pattern)
			end := start
			for end < len(description) && ((description[end] >= 'a' && description[end] <= 'z') ||
				(description[end] >= '0' && description[end] <= '9') ||
				description[end] == '-') {
				end++
			}
			if end > start {
				return strings.TrimSpace(description[start:end])
			}
		}
	}

	return ""
}

// UnblockDependents unblocks beads that were waiting on a decision
func (a *Loom) UnblockDependents(decisionID string) error {
	blocked := a.decisionManager.GetBlockedBeads(decisionID)

	for _, beadID := range blocked {
		if err := a.beadsManager.UnblockBead(beadID, decisionID); err != nil {
			return fmt.Errorf("failed to unblock bead %s: %w", beadID, err)
		}
	}

	return nil
}
