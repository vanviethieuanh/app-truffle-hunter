# truffle-hunter

Add a file name `queries.json`, with content like this:

```json
[
    {
        "search": "filename:.env TWILIO_AUTH_TOKEN",
        "description": "Searches for 'TWILIO_AUTH_TOKEN' in `.env` files, which may reveal Twilio authentication tokens."
    },
    ...
]
```

Run:

```sh
task
```

to run the tool, result will be saved to `result.jsonl`
