package suites

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"brale-core/internal/e2e"
)

func init() {
	e2e.Register("ctl", func() e2e.Suite { return &CTLSuite{} })
}

// CTLSuite tests bralectl CLI commands by actually invoking the binary.
type CTLSuite struct{}

func (s *CTLSuite) Name() string { return "ctl" }

func (s *CTLSuite) Run(ctx *e2e.Context) e2e.SuiteResult {
	start := time.Now()
	result := e2e.SuiteResult{
		Name:      s.Name(),
		StartedAt: start,
	}

	endpoint := ctx.Config.Endpoint
	symbol := ctx.Config.Symbol

	cmds := []struct {
		name                   string
		args                   []string
		suppressSuccessDetails bool
		validateOutput         func(string) error
	}{
		{"schedule status", []string{"schedule", "status", "--endpoint", endpoint}, false, nil},
		{"position list", []string{"position", "list", "--endpoint", endpoint}, false, nil},
		{"decision latest", []string{"decision", "latest", "--endpoint", endpoint, "--symbol", symbol}, false, nil},
		{"observe report", []string{"observe", "report", "--endpoint", endpoint, "--symbol", symbol}, true, validateObserveReportOutput},
	}

	selfBin, err := os.Executable()
	if err != nil {
		selfBin = "bralectl"
	}

	allPassed := true
	for _, cmd := range cmds {
		check := runCLICheck(selfBin, cmd.name, cmd.args, cmd.suppressSuccessDetails, cmd.validateOutput)
		result.Checks = append(result.Checks, check)
		if !check.Passed {
			allPassed = false
		}
	}

	// API-level healthz check
	healthCheck := e2e.CheckResult{Name: "healthz"}
	if err := ctx.Healthz(); err != nil {
		healthCheck.Passed = false
		healthCheck.Message = err.Error()
		allPassed = false
	} else {
		healthCheck.Passed = true
		healthCheck.Message = "ok"
	}
	result.Checks = append(result.Checks, healthCheck)

	// Market status check
	mktCheck := e2e.CheckResult{Name: "market/status " + symbol}
	mkt, err := ctx.MarketStatus(symbol)
	if err != nil {
		mktCheck.Passed = false
		mktCheck.Message = err.Error()
		allPassed = false
	} else {
		status, _ := mkt["status"].(string)
		mktCheck.Passed = status == "ok" || status == "unsupported"
		mktCheck.Message = fmt.Sprintf("status=%s", status)
		if !mktCheck.Passed {
			allPassed = false
		}
	}
	result.Checks = append(result.Checks, mktCheck)

	if allPassed {
		result.Status = e2e.StatusPass
	} else {
		result.Status = e2e.StatusFail
		result.Error = "one or more CLI checks failed"
	}
	result.Duration = time.Since(start)
	return result
}

func runCLICheck(bin, name string, args []string, suppressSuccessDetails bool, validateOutput func(string) error) e2e.CheckResult {
	check := e2e.CheckResult{Name: name}
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		check.Passed = false
		if len(outStr) > 200 {
			outStr = outStr[:200] + "..."
		}
		check.Message = fmt.Sprintf("exit=%v output=%s", err, outStr)
		return check
	}
	outStr := strings.TrimSpace(string(out))
	if validateOutput != nil {
		if validateErr := validateOutput(outStr); validateErr != nil {
			check.Passed = false
			check.Message = validateErr.Error()
			return check
		}
	}
	check.Passed = true
	if suppressSuccessDetails {
		check.Message = "ok"
		return check
	}
	check.Message = fmt.Sprintf("ok (%d bytes)", len(out))
	return check
}

func validateObserveReportOutput(out string) error {
	if strings.TrimSpace(out) == "" {
		return fmt.Errorf("empty observe report output")
	}
	return nil
}
