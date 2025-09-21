install -m 0755 -d /etc/apt/keyrings

tmpkey=$(mktemp)
if curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o "$tmpkey"; then
  if ! cmp -s "$tmpkey" /etc/apt/keyrings/docker.asc; then
    install -m 0644 "$tmpkey" /etc/apt/keyrings/docker.asc
  fi
fi
rm -f "$tmpkey"

. /etc/os-release
repo_line="deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $VERSION_CODENAME stable"
list_file=/etc/apt/sources.list.d/docker.list
if [ ! -f "$list_file" ] || ! grep -Fxq "$repo_line" "$list_file"; then
  printf '%s\n' "$repo_line" > "$list_file"
fi

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

usermod -aG docker deploy

docker --version
