I am looking at UnifiedArtifact, and two things seem super important to me:

1. Diffs
2. CommentTree

If a comment has a parent and is in reply to the bot - now the "context" is more isolated - because there is a relevant part of the thread, and probably some relevant part of the diff as well.

If a mention comment is attached to a particular diff line, then it also has more isolated diff

For a code review we may have to go file by file (or batches of files sometimes) - again to isolate the context

I think a natural way to deal with this is to introduce the idea of "ContextBatch" - as an intermediate step.

What this can do is - say - provide or focus more on the relevant context.

I think Diffs are foundational. From #file:types a diff is essentially identified via OldPath and NewPath. 

So I think the first step towards this ContextBatching is to go "file by file" through the UnifiedArtifact Diffs section. We need to be able to group by file paths. 

(Once we have the file paths -> We have relevant diffs -> Relevant Comments)

I want to think of these as "Focused file paths, focused diffs, and focused comments" these get highest priority. We can include rest of the stuff as well, but this must be "highlighted" for the LLM to reply intelligently to  a comment or line-based mention, etc
