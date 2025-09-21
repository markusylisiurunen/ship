cat > /etc/fail2ban/jail.local << EOF
[sshd]
enabled = true
bantime = 600
findtime = 300
maxretry = 8
EOF

systemctl status fail2ban

fail2ban-client reload
fail2ban-client status
fail2ban-client status sshd
