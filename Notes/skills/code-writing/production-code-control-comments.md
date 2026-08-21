---
name: production-code-control-comments
description: As an agent, how to protect production code from test code and other development artifacts.
---

In order to make the code safe for production, I use the following comments something like the C Preprocessor. You can look at the "clean-for-production.rb" program to see how this is implemented.

A line with a "// PRODUCTION:REMOVE" comment will be removed entirely. For example, a call to set up a route to allow seeding the database with test data should not be in the production code at all.

A line with a "// PRODUCTION:UNCOMMENT" comment, on the other hand, will be uncommented, and thus go into production. This is usually paired with a "// PRODUCTION:REMOVE" comment line that sets some paremeter to a testing value, and the uncomment version sets it to the production value.

Due to automatic typescript/javascript formatting, no lines ending in an open-brace (`{`) can get through the automatic formatter with a comment at the end, for example 'if' statements. Nor can a line ending in 'return ('. Accordingly, there is the "// PRODUCTION:REMOVE-NEXT-LINE" comment, which directs the cleanup program to remove the line following the one with the comment.

Finally, a line with a "// PRODUCTION:STOP" comment directs the cleanup program to stop processing lines entirely for the current file. This is to allow a function to remain, and be defined, but to have no behavior. So for example we might have the following function:

// demonstration only
export const foo = () => {
// } // PRODUCTION:UNCOMMENT
// PRODUCTION:STOP
   allowAnyoneToSignInWithAnyPassword()
   //...etc...
}

The typical thing is to have, as above, a "PRODUCTION:UNCOMMENT" line or lines that make whatever code is being defined syntactically correct, so the file can be imported and the function 'foo' called with no bad effects, but it will not do anything.
