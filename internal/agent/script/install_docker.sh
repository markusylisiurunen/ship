curl -fsSL https://get.docker.com | sed 's/set -x; sleep 20/set -x; sleep 1/g' | sh
usermod -aG docker deploy
