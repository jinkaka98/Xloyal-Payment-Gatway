FROM ghcr.io/m1k1o/neko/chromium:latest
COPY deploy/neko-chromium.conf /etc/neko/supervisord/chromium.conf
COPY deploy/cdp-proxy.py /opt/xloyal/cdp-proxy.py
COPY deploy/cdp-proxy.conf /etc/neko/supervisord/cdp-proxy.conf
RUN mkdir -p /home/neko/.config/chromium /home/neko/.cache && chown -R neko:neko /home/neko
