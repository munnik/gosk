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
// that can be stored, and neither message.DiagnosticRejected, which only a
// mapper sets, on a reading it rejected, nor message.DiagnosticDetails,
// which holds the mapping's DiagnosticDetails.
func configurable(name string) bool {
	return message.ValidDiagnosticName(name) && name != message.DiagnosticRejected && name != message.DiagnosticDetails
}

// precompileDiagnostics compiles a mapping's diagnostic and detail
// expressions once, at construction, next to its value expression (see
// precompileMapping). One that does not compile is logged and left out; it
// never stops the mapping itself.
func precompileDiagnostics(mc *config.MappingConfig) {
	if len(mc.Diagnostics) > 0 && mc.CompiledDiagnostics == nil {
		mc.CompiledDiagnostics = compileDiagnostics(mc.Diagnostics, configurable, mc.Path)
	}
	if len(mc.DiagnosticDetails) > 0 && mc.CompiledDiagnosticDetails == nil {
		mc.CompiledDiagnosticDetails = compileDiagnostics(mc.DiagnosticDetails, message.ValidDiagnosticName, mc.Path)
	}
}

func compileDiagnostics(sources map[string]string, allowed func(string) bool, path string) map[string]*vm.Program {
	compiled := make(map[string]*vm.Program, len(sources))
	for name, source := range sources {
		if !allowed(name) {
			logger.GetLogger().Error(
				"Ignoring a diagnostic whose name is reserved or cannot be stored",
				zap.String("Diagnostic", name),
				zap.String("Path", path),
			)
			continue
		}
		program, err := expr.Compile(source)
		if err != nil {
			logger.GetLogger().Error(
				"Could not compile a diagnostic expression",
				zap.String("Diagnostic", name),
				zap.String("Expression", source),
				zap.String("Path", path),
				zap.String("Error", err.Error()),
			)
			continue
		}
		compiled[name] = program
	}
	return compiled
}

// runDiagnostics evaluates every diagnostic of mc in env and returns the
// ones that produced a number, a bool or a string, with the details that
// did so under message.DiagnosticDetails, or nil for none.
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
	if len(mc.Diagnostics) == 0 && len(mc.DiagnosticDetails) == 0 {
		return nil
	}
	for key, value := range mc.ExpressionEnvironment {
		env[key] = value
	}

	result := evaluateDiagnostics(env, mc.Diagnostics, mc.CompiledDiagnostics, configurable, mc.Path)
	if details := evaluateDiagnostics(env, mc.DiagnosticDetails, mc.CompiledDiagnosticDetails, message.ValidDiagnosticName, mc.Path); len(details) > 0 {
		result[message.DiagnosticDetails] = details
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func evaluateDiagnostics(env ExpressionEnvironment, sources map[string]string, compiled map[string]*vm.Program, allowed func(string) bool, path string) map[string]any {
	result := make(map[string]any, len(sources))
	for name, source := range sources {
		if !allowed(name) {
			continue
		}
		program := compiled[name]
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
				zap.String("Path", path),
				zap.String("Error", err.Error()),
			)
			continue
		}
		result[name] = output
	}
	return result
}
