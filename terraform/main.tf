# Skeleton for a portfolio project - this is NOT applied against a real AWS
# account. A real user must supply vpc_id, subnet_ids, and ecr_repo_url (see
# variables.tf), plus db_password/jwt_secret via -var or an untracked tfvars
# file, before this would plan/apply successfully.

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# --- Networking / security groups ---

resource "aws_security_group" "alb" {
  name   = "taskflow-alb-${var.environment}"
  vpc_id = var.vpc_id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "ecs" {
  name   = "taskflow-ecs-${var.environment}"
  vpc_id = var.vpc_id

  ingress {
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  ingress {
    from_port       = 9090
    to_port         = 9090
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "rds" {
  name   = "taskflow-rds-${var.environment}"
  vpc_id = var.vpc_id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# --- Datastore ---

resource "aws_db_subnet_group" "taskflow" {
  name       = "taskflow-${var.environment}"
  subnet_ids = var.subnet_ids
}

resource "aws_db_instance" "taskflow" {
  identifier             = "taskflow-${var.environment}"
  engine                 = "postgres"
  engine_version         = "16"
  instance_class         = "db.t4g.micro"
  allocated_storage      = 20
  storage_encrypted      = true
  db_name                = "taskflow"
  username               = var.db_username
  password               = var.db_password
  db_subnet_group_name   = aws_db_subnet_group.taskflow.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  publicly_accessible    = false
  skip_final_snapshot    = true
}

# --- Secrets ---

resource "aws_secretsmanager_secret" "database_url" {
  name = "taskflow/${var.environment}/database-url"
}

resource "aws_secretsmanager_secret_version" "database_url" {
  secret_id     = aws_secretsmanager_secret.database_url.id
  secret_string = "postgres://${var.db_username}:${var.db_password}@${aws_db_instance.taskflow.endpoint}/taskflow?sslmode=require"
}

resource "aws_secretsmanager_secret" "jwt_secret" {
  name = "taskflow/${var.environment}/jwt-secret"
}

resource "aws_secretsmanager_secret_version" "jwt_secret" {
  secret_id     = aws_secretsmanager_secret.jwt_secret.id
  secret_string = var.jwt_secret
}

# --- ECS cluster ---

resource "aws_ecs_cluster" "taskflow" {
  name = "taskflow-${var.environment}"
}

resource "aws_iam_role" "execution" {
  name = "taskflow-ecs-execution-${var.environment}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "execution_secrets" {
  name = "taskflow-ecs-secrets-${var.environment}"
  role = aws_iam_role.execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = [aws_secretsmanager_secret.database_url.arn, aws_secretsmanager_secret.jwt_secret.arn]
    }]
  })
}

# --- Task definitions ---

locals {
  services = ["api", "worker", "scheduler"]

  container_secrets = [
    { name = "DATABASE_URL", valueFrom = aws_secretsmanager_secret.database_url.arn },
    { name = "JWT_SECRET", valueFrom = aws_secretsmanager_secret.jwt_secret.arn },
  ]
}

resource "aws_ecs_task_definition" "api" {
  family                   = "taskflow-api-${var.environment}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.execution.arn

  container_definitions = jsonencode([{
    name      = "api"
    image     = "${var.ecr_repo_url}/taskflow-api:${var.image_tag}"
    essential = true
    portMappings = [
      { containerPort = 8080, protocol = "tcp" },
      { containerPort = 9090, protocol = "tcp" },
    ]
    environment = [
      { name = "HTTP_ADDR", value = ":8080" },
      { name = "METRICS_ADDR", value = ":9090" },
    ]
    secrets = local.container_secrets
  }])
}

resource "aws_ecs_task_definition" "worker" {
  family                   = "taskflow-worker-${var.environment}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.execution.arn

  container_definitions = jsonencode([{
    name      = "worker"
    image     = "${var.ecr_repo_url}/taskflow-worker:${var.image_tag}"
    essential = true
    portMappings = [
      { containerPort = 9090, protocol = "tcp" },
    ]
    environment = [
      { name = "METRICS_ADDR", value = ":9090" },
    ]
    secrets = local.container_secrets
  }])
}

resource "aws_ecs_task_definition" "scheduler" {
  family                   = "taskflow-scheduler-${var.environment}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.execution.arn

  container_definitions = jsonencode([{
    name      = "scheduler"
    image     = "${var.ecr_repo_url}/taskflow-scheduler:${var.image_tag}"
    essential = true
    portMappings = [
      { containerPort = 9090, protocol = "tcp" },
    ]
    environment = [
      { name = "METRICS_ADDR", value = ":9090" },
    ]
    secrets = local.container_secrets
  }])
}

# --- ALB (api only - worker/scheduler don't serve traffic) ---

resource "aws_lb" "api" {
  name               = "taskflow-api-${var.environment}"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = var.subnet_ids
}

resource "aws_lb_target_group" "api" {
  name        = "taskflow-api-${var.environment}"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    path = "/healthz"
  }
}

resource "aws_lb_listener" "api" {
  load_balancer_arn = aws_lb.api.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
}

# --- ECS services ---

resource "aws_ecs_service" "api" {
  name            = "taskflow-api"
  cluster         = aws_ecs_cluster.taskflow.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = 2
  launch_type     = "FARGATE"

  network_configuration {
    subnets         = var.subnet_ids
    security_groups = [aws_security_group.ecs.id]
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "api"
    container_port   = 8080
  }

  depends_on = [aws_lb_listener.api]
}

resource "aws_ecs_service" "worker" {
  name            = "taskflow-worker"
  cluster         = aws_ecs_cluster.taskflow.id
  task_definition = aws_ecs_task_definition.worker.arn
  desired_count   = 2
  launch_type     = "FARGATE"

  network_configuration {
    subnets         = var.subnet_ids
    security_groups = [aws_security_group.ecs.id]
  }
}

resource "aws_ecs_service" "scheduler" {
  name            = "taskflow-scheduler"
  cluster         = aws_ecs_cluster.taskflow.id
  task_definition = aws_ecs_task_definition.scheduler.arn
  desired_count   = 2
  launch_type     = "FARGATE"

  network_configuration {
    subnets         = var.subnet_ids
    security_groups = [aws_security_group.ecs.id]
  }
}
