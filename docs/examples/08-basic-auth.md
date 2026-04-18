# Basic Authentication

Connect to APIs that use HTTP Basic Authentication (username and password). Common with legacy APIs, self-hosted services, and tools like Jenkins or Artifactory.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  jenkins.build_status:
    type: rest
    description: "Get the status of a Jenkins job"
    base_url: https://jenkins.internal.example.com
    method: GET
    path: /job/{{job_name}}/lastBuild/api/json
    auth:
      type: basic
      username: "{{vault:JENKINS_USER}}"
      password: "{{vault:JENKINS_API_TOKEN}}"
    parameters:
      - name: job_name
        in: path
        required: true

  jenkins.trigger_build:
    type: rest
    description: "Trigger a Jenkins build"
    base_url: https://jenkins.internal.example.com
    method: POST
    path: /job/{{job_name}}/build
    auth:
      type: basic
      username: "{{vault:JENKINS_USER}}"
      password: "{{vault:JENKINS_API_TOKEN}}"
    parameters:
      - name: job_name
        in: path
        required: true
```

## Usage

```bash
# Store credentials in the vault
factorly vault set JENKINS_USER admin
factorly vault set JENKINS_API_TOKEN 11a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1

# Check build status
factorly call jenkins.build_status --job_name my-pipeline

# Trigger a build
factorly call jenkins.trigger_build --job_name my-pipeline
```

## What happens

1. Factorly decrypts `JENKINS_USER` and `JENKINS_API_TOKEN` from the vault.
2. It Base64-encodes `admin:11a1a1a1...` and sends it as `Authorization: Basic YWRtaW46MTFhMWEx...`.
3. The response JSON is returned to your agent or terminal.
4. Neither the username nor the token appear in output, logs, or agent context.

---

[← Back to Examples](README.md)
