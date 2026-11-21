# Reminders and events

Create reminders by mentioning the bot:

```text
@Caffeinator remind me tomorrow at 4pm
@Caffeinator remind me every Monday at 9am to post the standup
@Caffeinator create an event for Friday at 3pm
```

Set your timezone first:

```text
/timezone zone:America/New_York
```

The bot posts a public announcement when a reminder is created. Other users can join by replying:

```text
me too
count me in
+1
```

They can leave with:

```text
leave
remove me
```

At the scheduled time, the bot mentions the creator and everyone who joined.

## Behavior

- Reminders are private in listings. Only the creator and participants see them.
- Events are public. Everyone sees active events in their list.
- Recurring schedules support daily, weekly, monthly, and intervals such as every other week.
- Times are computed in the user's timezone and stored in UTC. Discord timestamps display them locally.
- Missed reminders fire only if they are less than `REMINDER_GRACE_MINUTES` late, 60 minutes by default.
- Failed sends retry after `REMINDER_RETRY_MINUTES`, up to `REMINDER_MAX_ATTEMPTS` times.
- `/reminders` lists your reminders and public events, and lets you cancel or leave.
- `/reminders` responses are ephemeral.
- Each user can have up to `REMINDER_MAX_PER_USER` active reminders, 50 by default.
- The scheduler ticks every `REMINDER_TICK_SECONDS`, 15 by default.

## Storage

Reminders are stored as JSON:

```text
./reminders.json
```

Set `REMINDERS_PATH` to a persistent path in production, for example `/data/reminders.json`. Without a persistent path, reminders are lost on deploy.

Disable the feature with `REMINDERS_ENABLED=false`.
