package config

import "github.com/OpticDiff/code-reviewer/pkg/rules"

// Backwards-compatibility aliases for code that imports from internal/config.
// The canonical API is now in pkg/rules/.
type RuleRouter = rules.RuleRouter
type RuleLoader = rules.RuleLoader

var LoadRuleRouter = rules.LoadRuleRouter
var NewRuleLoader = rules.NewRuleLoader
