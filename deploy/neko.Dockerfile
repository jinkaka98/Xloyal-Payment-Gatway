FROM ghcr.io/m1k1o/neko/chromium:latest
COPY deploy/neko-chromium.conf /etc/neko/supervisord/chromium.conf
COPY deploy/cdp-proxy.conf /etc/neko/supervisord/cdp-proxy.conf
COPY deploy/neko-helper.py /opt/xloyal/neko-helper.py
COPY deploy/neko-helper.conf /etc/neko/supervisord/neko-helper.conf
RUN apt-get update && apt-get install -y --no-install-recommends socat xclip xdotool && rm -rf /var/lib/apt/lists/* && \
    mkdir -p /home/neko/.config/chromium /home/neko/.cache && chown -R neko:neko /home/neko
