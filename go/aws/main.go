package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	didSignIn, err := signinWithGitpod()
	if didSignIn {
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "login using Gitpod IDP failed: %v", err)
	}

	didSignIn, err = signinWithSSO()
	if didSignIn {
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "login using SSO failed: %v", err)
	}

	fmt.Fprintf(os.Stderr, "don't know how to sign in - I've tried everythin 🤷\n")
}

func signinWithGitpod() (didSignIn bool, err error) {
	if !runningInGitpod() {
		return false, nil
	}

	if os.Getenv("IDP_AWS_ROLE_ARN") == "" {
		fmt.Fprintf(os.Stderr, "Running in a Gitpod workspace, but the IDP_AWS_ROLE_ARN environment variable is not set.\nPlease setup OIDC trust (https://www.gitpod.io/docs/integrations/aws) and set the IDP_AWS_ROLE_ARN environment variable on your project\n\n")
		return false, nil
	}

	out, err := exec.Command("gp", "idp", "login", "aws").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("gp idp login failure: %s: %w", string(out), err)
	}

	return true, nil
}

func runningInGitpod() bool {
	if os.Getenv("GITPOD_WORKSPACE_URL") == "" {
		return false
	}
	if pth, _ := exec.LookPath("gp"); pth == "" {
		return false
	}
	return true
}

func signinWithSSO() (didSignIn bool, err error) {
	// NOTE(cw): only here for demo purposes - no need to implement this
	return false, nil
}
