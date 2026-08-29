terraform {
  required_providers {
    aiprovider = {
      source  = "registry.example.com/ai/ai"
      version = "0.1.0"
    }
  }
}

provider "aiprovider" {
  endpoint  = "http://localhost:8080"
  api_token = "test-token" # optional; falls back to AIPROVIDER_API_TOKEN
}

resource "aiprovider_cluster" "demo" {
  name     = "demo-cluster"
  replicas = 3
  model    = "gpt-mini"
}

resource "aiprovider_job" "demo" {
  name     = "demo-job"
  command  = "echo hello"
  priority = 5
}

data "aiprovider_cluster" "by_id" {
  id = aiprovider_cluster.demo.id
}

output "cluster_id" {
  value = aiprovider_cluster.demo.id
}

output "job_id" {
  value = aiprovider_job.demo.id
}

output "job_status" {
  value = aiprovider_job.demo.status
}

output "cluster_name_from_data_source" {
  value = data.aiprovider_cluster.by_id.name
}
