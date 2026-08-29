# dev.ps1 - Docker dev-workflow helpers for Windows PowerShell.
#
# Mirror the key Makefile targets for machines without `make`. Load with:
#   . ./dev.ps1

$ErrorActionPreference = 'Stop'

$IMAGE      = 'ai-dev'
$DOCKERFILE = 'Dockerfile'
$EXAMPLES   = "$PWD/examples"

# --- Mock API server (docker compose) ---

# Build and start the mock API (HTTP :8080, gRPC :9090).
function Invoke-MockUp {
    docker compose up -d --build
}

# Show mock API logs.
function Invoke-MockLogs {
    docker compose logs -f
}

# Stop and remove the mock API.
function Invoke-MockDown {
    docker compose down
}

# --- Dev image + terraform in Docker ---

# Build the dev image (Terraform + provider installed).
function Invoke-DevBuild {
    docker build --target dev -f $DOCKERFILE -t $IMAGE .
}

# Run any terraform command, e.g. Invoke-DevTf validate | plan | 'apply -auto-approve'.
function Invoke-DevTf {
    param([string]$Cmd = 'plan')
    docker run --rm --entrypoint sh -v "${EXAMPLES}:/work" $IMAGE -c "terraform init -input=false && terraform $Cmd -input=false"
}

# Validate examples/main.tf.
function Invoke-DevValidate {
    Invoke-DevTf 'validate'
}

# Show the plan for examples/main.tf.
function Invoke-DevPlan {
    Invoke-DevTf 'plan'
}

# Build the dev image, then validate + plan.
function Invoke-Dev {
    Invoke-DevBuild
    Invoke-DevValidate
    Invoke-DevPlan
}
