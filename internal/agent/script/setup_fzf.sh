if [ -d "/root/.fzf" ]; then
  echo "[root] ~/.fzf exists, skipping clone/install"
else
  echo "[root] Cloning fzf"
  git clone --depth 1 https://github.com/junegunn/fzf.git /root/.fzf
fi
# cd /root/.fzf && git pull
/root/.fzf/install --all --no-zsh --no-fish

if [ -d "/home/deploy/.fzf" ]; then
  echo "[deploy] ~/.fzf exists, skipping clone/install"
else
  echo "[deploy] Cloning fzf"
  sudo -u deploy git clone --depth 1 https://github.com/junegunn/fzf.git /home/deploy/.fzf
fi
# cd /home/deploy/.fzf && sudo -u deploy git pull
sudo -u deploy /home/deploy/.fzf/install --all --no-zsh --no-fish
