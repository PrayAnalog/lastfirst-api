// Build with `docker buildx bake -f backend/docker-bake.hcl --push` from the
// project root. This always targets linux/amd64 (DOKS node architecture),
// so the platform doesn't depend on --platform being passed by hand.

group "default" {
  targets = ["backend"]
}

target "backend" {
  context    = "."
  dockerfile = "backend/Dockerfile"
  platforms  = ["linux/amd64"]
  tags       = ["registry.digitalocean.com/lastfirst/backend:v1"]
}
