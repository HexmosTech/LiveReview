A report returned zero rows. Tell the user plainly that there is nothing to
show, in one or two short sentences.

Reply with a single JSON object and nothing else:

```
{"response_type": "no_data", "text": "Hexmos completed no reviews today."}
```

Name the organization and the time period the user asked about. Do not
apologize, do not speculate about why the data is empty, and do not suggest
that something went wrong — zero is a legitimate answer.
