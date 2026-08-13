import asyncio

UPSTREAM_HOST = b"127.0.0.1:9223"
PUBLIC_HOST = b"neko:9222"


async def _pipe(source, destination):
    """Copy one half of a CDP connection until EOF or disconnect."""
    try:
        while True:
            data = await source.read(65536)
            if not data:
                return
            destination.write(data)
            await destination.drain()
    except (ConnectionError, OSError):
        return


async def bridge(reader, writer):
    upstream_writer = None
    tasks = set()
    try:
        upstream_reader, upstream_writer = await asyncio.open_connection("127.0.0.1", 9223)

        # Chromium's DevTools server validates the HTTP Host header. Docker
        # clients use ``neko:9222``, so translate it before the HTTP/WebSocket
        # handshake and translate the discovery URL back for chromedp.
        request = await reader.readuntil(b"\r\n\r\n")
        request = _replace_host_header(request, UPSTREAM_HOST)
        upstream_writer.write(request)
        await upstream_writer.drain()

        response = await upstream_reader.readuntil(b"\r\n\r\n")
        content_length = _content_length(response)
        body = await upstream_reader.readexactly(content_length) if content_length else b""
        if body:
            body = body.replace(b"127.0.0.1:9223", PUBLIC_HOST)
            response = _set_content_length(response, len(body))
        writer.write(response + body)
        await writer.drain()

        tasks = {
            asyncio.create_task(_pipe(reader, upstream_writer)),
            asyncio.create_task(_pipe(upstream_reader, writer)),
        }
        # A CDP peer is done when either direction closes.  Cancel the other
        # task immediately so sockets and task references cannot accumulate.
        await asyncio.wait(tasks, return_when=asyncio.FIRST_COMPLETED)
    except (ConnectionError, OSError):
        pass
    finally:
        for task in tasks:
            if not task.done():
                task.cancel()
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)
        writer.close()
        await writer.wait_closed()
        if upstream_writer is not None:
            upstream_writer.close()
            await upstream_writer.wait_closed()


def _replace_host_header(headers, host):
    lines = headers.split(b"\r\n")
    for index, line in enumerate(lines):
        if line.lower().startswith(b"host:"):
            lines[index] = b"Host: " + host
            break
    return b"\r\n".join(lines)


def _content_length(headers):
    for line in headers.split(b"\r\n"):
        if line.lower().startswith(b"content-length:"):
            return int(line.split(b":", 1)[1].strip())
    return 0


def _set_content_length(headers, length):
    lines = headers.split(b"\r\n")
    for index, line in enumerate(lines):
        if line.lower().startswith(b"content-length:"):
            lines[index] = b"Content-Length: " + str(length).encode()
            break
    return b"\r\n".join(lines)


async def main():
    server = await asyncio.start_server(bridge, "0.0.0.0", 9222, limit=65536)
    async with server:
        await server.serve_forever()


asyncio.run(main())
