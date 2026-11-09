resource "aws_lb" "ingress" {
  name                       = var.name
  internal                   = false
  load_balancer_type         = "application"
  security_groups            = [aws_security_group.alb.id]
  subnets                    = var.public_subnet_ids
  enable_deletion_protection = true
  drop_invalid_header_fields = true
  idle_timeout               = 120
}
resource "aws_lb_target_group" "web" {
  name     = "${var.name}-web"
  port     = 42069
  protocol = "HTTP"
  vpc_id   = var.vpc_id
  health_check {
    path    = "/signed-out"
    matcher = "200"
  }
}
resource "aws_lb_target_group" "gateway" {
  name     = "${var.name}-gateway"
  port     = 8081
  protocol = "HTTP"
  vpc_id   = var.vpc_id
  health_check {
    path    = "/__sg1_health"
    matcher = "200"
  }
}
resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.ingress.arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}
resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.ingress.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate_validation.ingress.certificate_arn
  default_action {
    type = "fixed-response"
    fixed_response {
      content_type = "text/plain"
      message_body = "Not found"
      status_code  = "404"
    }
  }
}
resource "aws_lb_listener_rule" "signed_out" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 5
  condition {
    host_header { values = [var.domain] }
  }
  condition {
    path_pattern { values = ["/signed-out"] }
  }
  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.web.arn
  }
}
resource "aws_lb_listener_rule" "employees" {
  for_each     = { pages = { priority = 10, paths = ["/", "/build/*", "/settings/*"] }, api = { priority = 11, paths = ["/api/*", "/login"] } }
  listener_arn = aws_lb_listener.https.arn
  priority     = each.value.priority
  condition {
    host_header { values = [var.domain] }
  }
  condition {
    path_pattern { values = each.value.paths }
  }
  action {
    type  = "authenticate-cognito"
    order = 1
    authenticate_cognito {
      user_pool_arn              = aws_cognito_user_pool.employees.arn
      user_pool_client_id        = aws_cognito_user_pool_client.alb.id
      user_pool_domain           = aws_cognito_user_pool_domain.employees.domain
      session_cookie_name        = "SG1AuthSession"
      session_timeout            = 3600
      scope                      = "openid email profile"
      on_unauthenticated_request = "authenticate"
    }
  }
  action {
    type             = "forward"
    order            = 2
    target_group_arn = aws_lb_target_group.web.arn
  }
}
resource "aws_lb_listener_rule" "viewers" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 15
  condition {
    host_header { values = [var.domain] }
  }
  # /{project-slug} and static assets are public; the isolated gateway enforces
  # visitor access, while internal APIs independently verify signed identity.
  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.web.arn
  }
}
resource "aws_lb_listener_rule" "apps" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 20
  condition {
    host_header { values = ["*.${var.apps_domain}"] }
  }
  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.gateway.arn
  }
}
resource "aws_route53_record" "app" {
  for_each = toset([var.domain, "*.${var.apps_domain}"])
  zone_id  = each.value == var.domain ? var.hosted_zone_id : var.apps_hosted_zone_id
  name     = each.value
  type     = "A"
  alias {
    name                   = aws_lb.ingress.dns_name
    zone_id                = aws_lb.ingress.zone_id
    evaluate_target_health = true
  }
}
resource "aws_instance" "control" {
  ami                         = data.aws_ssm_parameter.ami.value
  instance_type               = var.instance_type
  subnet_id                   = var.private_subnet_ids[0]
  vpc_security_group_ids      = [aws_security_group.control.id]
  associate_public_ip_address = false
  iam_instance_profile        = aws_iam_instance_profile.control.name
  metadata_options {
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
  }
  root_block_device {
    encrypted             = true
    volume_type           = "gp3"
    volume_size           = 40
    delete_on_termination = false
  }
  user_data = file("${path.module}/user-data.sh")
  tags      = { Name = "${var.name}-control" }
  lifecycle {
    precondition {
      condition     = alltrue([for subnet in data.aws_subnet.private : subnet.vpc_id == var.vpc_id]) && length(toset([for subnet in data.aws_subnet.private : subnet.availability_zone])) >= 2 && data.aws_vpc.existing.enable_dns_support && data.aws_vpc.existing.enable_dns_hostnames
      error_message = "Private subnets must belong to the VPC, with DNS support/hostnames enabled."
    }
  }
}
resource "aws_lb_target_group_attachment" "web" {
  target_group_arn = aws_lb_target_group.web.arn
  target_id        = aws_instance.control.id
}
resource "aws_lb_target_group_attachment" "gateway" {
  target_group_arn = aws_lb_target_group.gateway.arn
  target_id        = aws_instance.control.id
}
