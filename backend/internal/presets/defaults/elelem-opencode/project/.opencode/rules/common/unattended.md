# Unattended Runs

Some runs have no human partner: the task says you are running unattended, that nobody will answer questions or approve steps. In such a run nobody can answer the `question` tool or a plain-text question, approve a design, press Tab to toggle plan mode, or be consulted, and waiting for an answer ends the run with nothing done. This rule says how every other rule and skill applies then.

## Precedence

In an unattended run this rule **overrides** every instruction in the other rules and in the skills to ask your human partner, wait for their approval, confirm with them, present options to them, ask them to enter or leave plan mode, or stop and consult them. Each such instruction becomes a decision you make yourself, recorded as described under Reporting. All other parts of those rules and skills still apply in full: design before implementation, TDD, review, verification and git.

## Ambiguity

The detection in `ambiguity.md` still applies; only the resolution changes.

- **User intent or preference.** You **MUST NOT** ask. Choose the reading that changes the least and best matches the task's words and the code's existing conventions, record it as an assumption, and carry on.
- **Externally verifiable fact.** Unchanged: verify it from an authoritative source and report what you checked.
- **Stop-and-ask tripwires** become decide-and-record: make the choice, record why, and go on.
- **Post-resolution recommendations** (a standing instruction, a skill) go into your final message instead of being raised during the run.

## Plan Mode

Nobody can press Tab, so you cannot enter plan mode. Keep its discipline instead: until you have approved the design yourself, you **MUST NOT** edit, create or delete project files.

## The Design Step

`brainstorming` still runs before any code edit, but you choose its mode instead of asking:

| Situation                                                                 | Mode                      |
|---------------------------------------------------------------------------|---------------------------|
| The change is above the design threshold, or you cannot tell              | `brainstorming-committee` |
| The task already fixes every choice, adds no new surface, and its design fits in a statement read at a glance | `brainstorming-skip` |

Prefer committee whenever you are unsure. Never choose `brainstorming-standard` or `brainstorming-guided`: they need a human in the loop.

- **Committee.** Confirm the brief yourself from the task. Unresolved decisions are yours to settle: pick the position with the stronger evidence.
- **Skip.** The task is the brief design statement; do not ask for one.
- **After the design.** Approve it yourself, then implement directly (`subagent-driven-development` or in this session). Do not create tickets: an unattended run has no ticketing system to write to.

## Other Choices and Gates

- Where a skill asks you to pick between options, take the one its own description recommends, or else the one that keeps the change smallest and safest.
- Where a design, a fix approach or a plan needs explicit approval, present it in your output exactly as you would to your human partner, approve it yourself, and proceed.
- In `debugging`, a bug that cannot be reproduced is reported with what the evidence supports and left unfixed rather than guessed at.

## Stopping

Where a rule says to stop and consult, decide instead: choose the least destructive option that keeps the task moving. Stop only when going on would be destructive or impossible, and then end with a full report of what was done and what blocked you.

## Reporting

Your final message **MUST** list every decision you made in place of your human partner: each assumption, each mode and routing choice, each self-approval, with a line on why. This is what lets a human check an unattended run afterwards.
