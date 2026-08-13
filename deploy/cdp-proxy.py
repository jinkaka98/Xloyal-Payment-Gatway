import asyncio


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


async def main():
    server = await asyncio.start_server(bridge, "0.0.0.0", 9222, limit=65536)
    async with server:
        await server.serve_forever()


asyncio.run(main())
