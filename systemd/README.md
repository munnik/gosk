install service:

sudo rm /lib/systemd/system/gosk_*.service && \
sudo ln -s ~/Code/gosk/systemd/gosk_*.service /lib/systemd/system && \
sudo chmod 755 /lib/systemd/system/gosk_*.service && \
sudo systemctl enable /lib/systemd/system/gosk_*.service && \
sudo systemctl daemon-reload