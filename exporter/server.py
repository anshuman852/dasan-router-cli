"""Prometheus HTTP server that periodically scrapes the Dasan router.

Usage::

    python -m exporter.server --host 192.168.1.1 --username admin --password secret

Or set ``DASAN_HOST`` / ``DASAN_USERNAME`` / ``DASAN_PASSWORD`` env vars::

    set DASAN_HOST=192.168.1.1
    set DASAN_USERNAME=admin
    set DASAN_PASSWORD=secret
    python -m exporter.server
"""
import argparse
import logging
import os
import sys
import threading
import time

from prometheus_client import start_http_server

from dasan_cli.core import DasanClient
from exporter.exporter import DasanMetricsCollector

logger = logging.getLogger(__name__)


def resolve_credentials(args):
    """Return ``(host, username, password)`` from CLI args or env vars."""
    host = args.host or os.environ.get("DASAN_HOST", "192.168.1.1")
    username = (
        args.username
        or os.environ.get("DASAN_USERNAME")
        or os.environ.get("DASAN_USER")
    )
    password = (
        args.password
        or os.environ.get("DASAN_PASSWORD")
        or os.environ.get("DASAN_PASS")
    )
    return host, username, password


def build_parser():
    parser = argparse.ArgumentParser(
        description="Prometheus exporter for Dasan H660GM-A GPON router",
    )
    parser.add_argument(
        "--host", default=None,
        help="Router IP (default: $DASAN_HOST or 192.168.1.1)",
    )
    parser.add_argument(
        "--username", default=None,
        help="Router login username (default: $DASAN_USERNAME or $DASAN_USER)",
    )
    parser.add_argument(
        "--password", default=None,
        help="Router login password (default: $DASAN_PASSWORD or $DASAN_PASS)",
    )
    parser.add_argument(
        "--port", type=int, default=9800,
        help="Exporter HTTP listen port (default: 9800)",
    )
    parser.add_argument(
        "--interval", type=int, default=60,
        help="Scrape interval in seconds (default: 60)",
    )
    parser.add_argument(
        "--log-level", default="INFO",
        choices=["DEBUG", "INFO", "WARNING", "ERROR"],
        help="Logging verbosity (default: INFO)",
    )
    return parser


def main():
    parser = build_parser()
    args = parser.parse_args()

    logging.basicConfig(
        level=getattr(logging, args.log_level.upper(), logging.INFO),
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
        datefmt="%Y-%m-%dT%H:%M:%S",
    )

    # -- Credentials --------------------------------------------------------
    host, username, password = resolve_credentials(args)
    if not username or not password:
        logger.error(
            "Username and password are required. Provide via --username/--password "
            "or DASAN_USERNAME/DASAN_PASSWORD environment variables."
        )
        sys.exit(1)

    # -- Login --------------------------------------------------------------
    logger.info("Connecting to router at %s ...", host)
    client = DasanClient(host)
    client.login(username, password)
    logger.info("Logged in successfully")

    # -- Initial scrape -----------------------------------------------------
    collector = DasanMetricsCollector(client)
    try:
        collector.collect()
        logger.info("Initial scrape completed")
    except Exception as e:
        logger.error("Initial scrape failed: %s", e)

    # -- HTTP server --------------------------------------------------------
    start_http_server(args.port)
    logger.info("Prometheus metrics listening on :%d", args.port)

    # -- Background collection loop -----------------------------------------
    stop_event = threading.Event()

    def collect_loop():
        while not stop_event.is_set():
            stop_event.wait(args.interval)
            if stop_event.is_set():
                break
            try:
                collector.collect()
            except Exception as e:
                logger.error("Collection cycle failed: %s", e)

    thread = threading.Thread(target=collect_loop, daemon=True)
    thread.start()
    logger.info(
        "Background collection every %d s (slow objects every %d s)",
        args.interval,
        300,
    )

    # -- Wait for shutdown --------------------------------------------------
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        logger.info("Shutting down ...")
        stop_event.set()


if __name__ == "__main__":
    main()
