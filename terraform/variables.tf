variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "environment" {
  type    = string
  default = "dev"
}

# A real user must supply these three - there is no sane default for someone
# else's VPC/subnets/registry.
variable "vpc_id" {
  type = string
}

variable "subnet_ids" {
  type = list(string)
}

variable "ecr_repo_url" {
  description = "Base ECR repo URL, e.g. <account_id>.dkr.ecr.<region>.amazonaws.com/taskflow"
  type        = string
}

variable "image_tag" {
  type    = string
  default = "latest"
}

variable "db_username" {
  type    = string
  default = "taskflow"
}

variable "db_password" {
  description = "RDS master password. Supply via -var or a tfvars file that is never committed."
  type        = string
  sensitive   = true
}

variable "jwt_secret" {
  description = "JWT signing secret for the api service. Supply via -var or a tfvars file that is never committed."
  type        = string
  sensitive   = true
}
