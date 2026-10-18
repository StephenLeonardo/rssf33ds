### Reconstructing relationships between tasks

Instead of unwinding each task’s stack and reporting it independently—which is technically the most faithful way to do it, but results in the issues above—we keep track of parent-child relationships between tasks: which task is awaiting which. 

When unwinding, we first build the frame stack for each task, then build a task dependency tree from those relationships. Finally, we use that tree to create the stacked stacks we report. By stacking related tasks, we can show what each coroutine is blocked on, representing those relationships much like function calls in synchronous programming. 

The result is that programs that structure the same work in different ways produce comparable flame graphs. First, consider an asyncio implementation in which the work is split across separate tasks:

```

1
# Two separate tasks for check_auth and cassandra queries

2
import aio_cassandra

3

4
async def validate_user() -> None:

5
 # (... handle caching ...)

6

7
 cass_query = asyncio.create_task(aio_cassandra.query(...))

8
 # cass_query has a separate execution context and stack

9
 await cass_query

10

11
 # send metrics in the background

12
 asyncio.create_task(send_metrics())

13

14
 # (... check, then return or raise ...)

15

16
async def check_auth() -> None:

17
 # raise if not logged in

18
 await validate_user()

19

20
asyncio.run(check_auth())
```
