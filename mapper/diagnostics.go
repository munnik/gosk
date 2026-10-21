package mapper

import (
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"
)

// configurable reports whether a mapping may name a diagnostic name: one
// that can be stored, and not message.DiagnosticRejected, which only a
// mapper sets, on a reading it rejected.
func configurable(name string) bool {
	return message.ValidDiagnosticName(name) && name != message.DiagnosticRejected
}

// precompileDiagnostics compiles a mapping's diagnostic expressions once,
// at construction, next to its value expression (see precompileMapping).
// A diagnostic that does not compile is logged and left out; it never stops
// the mapping itself.
func precompileDiagnostics(mc *config.MappingConfig) {
	if len(mc.Diagnostics) == 0 || mc.CompiledDiagnostics != nil {
		return
	}
	mc.CompiledDiagnostics = make(map[string]*vm.Program, len(mc.Diagnostics))
	for name, source := range mc.Diagnostics {
		if !configurable(name) {
			logger.GetLogger().Error(
				"Ignoring a diagnostic whose name is reserved or cannot be stored",
				zap.String("Diagnostic", name),
				zap.String("Path", mc.Path),
			)
			continue
		}
		program, err := expr.Compile(source)
		if err != nil {
			logger.GetLogger().Error(
				"Could not compile a diagnostic expression",
				zap.String("Diagnostic", name),
				zap.String("Expression", source),
				zap.String("Path", mc.Path),
				zap.String("Error", err.Error()),
			)
			continue
		}
		mc.CompiledDiagnostics[name] = program
	}
}

// runDiagnostics evaluates every diagnostic of mc in env and returns the
// ones that produced a number, a bool or a string, or nil for none.
//
// One that fails is left out of this reading and logged at debug only:
// a diagnostic describes a reading, so failing to describe one - a status
// register not polled yet on the first reply, say - must never cost the
// value itself, and a warning per poll would bury the journal.
//
// It only reads the programs precompileDiagnostics stored. Compiling one
// here and storing it would write to a mapping config that sharded mappers
// share between goroutines, so a diagnostic left uncompiled by a mapper
// that never precompiled is compiled for this call alone.
func runDiagnostics(env ExpressionEnvironment, mc *config.MappingConfig) map[string]any {
	if len(mc.Diagnostics) == 0 {
		return nil
	}
	for key, value := range mc.ExpressionEnvironment {
		env[key] = value
	}

	result := make(map[string]any, len(mc.Diagnostics))
	for name, source := range mc.Diagnostics {
		if !configurable(name) {
			continue
		}
		program := mc.CompiledDiagnostics[name]
		if program == nil {
			var err error
			if program, err = expr.Compile(source); err != nil {
				continue
			}
		}
		output, err := runVM(program, env)
		if err == nil {
			output, err = message.NormalizeDiagnostic(output)
		}
		if err != nil {
			logger.GetLogger().Debug(
				"Leaving out a diagnostic that did not evaluate",
				zap.String("Diagnostic", name),
				zap.String("Path", mc.Path),
				zap.String("Error", err.Error()),
			)
			continue
		}
		result[name] = output
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
