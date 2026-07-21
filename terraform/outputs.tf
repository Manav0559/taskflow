output "alb_dns_name" {
  value = aws_lb.api.dns_name
}

output "rds_endpoint" {
  value = aws_db_instance.taskflow.endpoint
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.taskflow.name
}
