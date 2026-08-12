import asyncio
from mcp import ClientSession, StdioServerParameters
from mcp.client.sse import sse_client

async def main():
    async with sse_client("http://127.0.0.1:19080/sse") as (read, write):
        async with ClientSession(read, write) as session:
            init = await session.initialize()
            print("server:", init.serverInfo.name, init.serverInfo.version)
            tools = await session.list_tools()
            names = [t.name for t in tools.tools]
            print("tools (%d):" % len(names), names)
            # call list_hosts
            res = await session.call_tool("list_hosts", {"online_only": "true"})
            print("list_hosts:", res.content[0].text[:200])
            # call execute_command
            res = await session.call_tool("execute_command", {"host_id": "2", "command": "id; uname -m"})
            print("execute_command:", res.content[0].text[:250])
            # call list_directory
            res = await session.call_tool("list_directory", {"host_id": "2", "path": "/etc"})
            print("list_directory:", res.content[0].text[:250])
            # call read_file
            res = await session.call_tool("write_file", {"host_id":"2","path":"/tmp","filename":"sdk_test.txt","content":"hello from sdk\n"})
            print("write_file:", res.content[0].text[:200])
            res = await session.call_tool("read_file", {"host_id":"2","path":"/tmp","filename":"sdk_test.txt"})
            print("read_file:", res.content[0].text[:250])

asyncio.run(main())
