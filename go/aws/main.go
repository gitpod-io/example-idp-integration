package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"
)

type SigninMethodFunc func() (didSignIn bool, err error)

func main() {
	var signinMethods = []SigninMethodFunc{
		signinWithGitpod,
		signinWithGitpodVerbose,
		signinWithSSO,
	}
	for _, signin := range signinMethods {
		didSignIn, err := signin()
		if didSignIn {
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error while logging in: %v\n", err)
		}
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

// signinWithGitpodVerbose demonstrates how Gitpod's APIs can be used without the gp CLI.
//
// Note: this is considerably more brittle than using the gp CLI, as some of the APIs are not entirely stable yet and may change without prior notice.
func signinWithGitpodVerbose() (didSignIn bool, err error) {
	if !runningInGitpod() {
		return false, nil
	}
	roleARN := os.Getenv("IDP_AWS_ROLE_ARN")
	if roleARN == "" {
		fmt.Fprintf(os.Stderr, "Running in a Gitpod workspace, but the IDP_AWS_ROLE_ARN environment variable is not set.\nPlease setup OIDC trust (https://www.gitpod.io/docs/integrations/aws) and set the IDP_AWS_ROLE_ARN environment variable on your project\n\n")
		return false, nil
	}

	// 1. Get token to talk to Gitpod
	var (
		supervisorAddr = os.Getenv("SUPERVISOR_ADDR")
		gitpodHostRaw  = os.Getenv("GITPOD_HOST")
		workspaceID    = os.Getenv("GITPOD_WORKSPACE_ID")
	)
	gitpodHost, err := url.Parse(gitpodHostRaw)
	if err != nil {
		return false, fmt.Errorf("invalid Gitpod host url: %w", err)
	}
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/_supervisor/v1/token/gitpod/%s/", supervisorAddr, gitpodHost.Host))
	if err != nil {
		return false, fmt.Errorf("cannot get gitpod token: %w", err)
	}
	defer resp.Body.Close()
	var tkn struct {
		Token string `json:"token"`
	}
	err = json.NewDecoder(resp.Body).Decode(&tkn)
	if err != nil {
		return false, fmt.Errorf("cannot decode gitpod token: %w", err)
	}

	// 2. Produce identity token
	idpReq, err := json.Marshal(struct {
		WorkspaceID string   `json:"workspace_id"`
		Audience    []string `json:"audience"`
	}{
		WorkspaceID: workspaceID,
		Audience:    []string{"sts.amazonaws.com"},
	})
	if err != nil {
		return false, fmt.Errorf("cannot marshal ID token request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("https://api.%s/gitpod.experimental.v1.IdentityProviderService/GetIDToken", gitpodHost.Host), bytes.NewReader(idpReq))
	if err != nil {
		return false, fmt.Errorf("cannot prepare ID token request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tkn.Token))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		return false, fmt.Errorf("cannot make ID token request: %w", err)
	}
	defer resp.Body.Close()
	var idtkn struct {
		Token string `json:"token"`
	}
	err = json.NewDecoder(resp.Body).Decode(&tkn)
	if err != nil {
		return false, fmt.Errorf("cannot decode ID token response: %w", err)
	}

	// 3. Exchange ID token for AWS credentials
	awsCmd := exec.Command("aws", "sts", "assume-role-with-web-identity", "--role-arn", roleARN, "--role-session-name", fmt.Sprintf("%s-%d", workspaceID, time.Now().Unix()), "--web-identity-token", idtkn.Token)
	out, err := awsCmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%w: %s", err, string(out))
	}

	// 4. Persist credentials as AWS profile
	var result struct {
		Credentials struct {
			AccessKeyId     string
			SecretAccessKey string
			SessionToken    string
		}
	}
	err = json.Unmarshal(out, &result)
	if err != nil {
		return false, err
	}
	vars := map[string]string{
		"aws_access_key_id":     result.Credentials.AccessKeyId,
		"aws_secret_access_key": result.Credentials.SecretAccessKey,
		"aws_session_token":     result.Credentials.SessionToken,
	}
	for k, v := range vars {
		awsCmd := exec.Command("aws", "configure", "set", "--profile", "default", k, v)
		out, err := awsCmd.CombinedOutput()
		if err != nil {
			return false, fmt.Errorf("%w: %s", err, string(out))
		}
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
