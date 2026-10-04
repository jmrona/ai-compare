# Unattended Runs

Some runs have no human partner: the task says you are running unattended, that nobody will answer questions or approve steps. In such a run nobody can answer `AskUserQuestion`, approve a design, or be consulted, and waiting for an answer ends the run with nothing done. This rule says how every other rule and skill applies then.

## Precedence

In an unattended run this rule **overrides** every instruction in the other rules and in the skills to ask your human partner, wait for their approval, confirm with them, present options to them, or stop and consult them. Each such instruction becomes a decision you make yourself, recorded as described under Reporting. All other parts of those rules and skills still apply in full: design before implementation, TDD, review, verification and git.

## Ambiguity

The detection in `ambiguity.md` still applies; only the resolution changes.

- **User intent or preference.** You **MUST NOT** ask. Choose the reading that changes the least and best matches the task's words and the code's existing conventions, record it as an assumption, and carry on.
- **Externally verifiable fact.** Unchanged: verify it from an authoritative source and report what you checked.
- **Stop-and-ask tripwires** become decide-and-record: make the choice, record why, and go on.

## Routing Asks

Where a rule or a skill tells you to put a routing choice to your human partner, choose it yourself:

| Choice                                   | Choose                                                                                                                                       |
|------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| How to run the design step               | The **committee** skill (hands-off deliberation by subagents, such as `design-committee` or `brainstorming-committee`) when the change is above the design threshold in `workflow.md`; the **skip** path (a short design statement, or `brainstorming-skip`) when all three threshold criteria hold. Prefer committee whenever you are unsure. |
| Interactive design skills                | Never: `design-dialogue`, `brainstorming-standard` and `brainstorming-guided` need a human in the loop.                                       |
| What happens after the design           | "Implement directly". Do not create tickets: an unattended run has no ticketing system to write to.                                          |
| A skill that asks which of several options | The option its own description recommends, or else the one that keeps the change smallest and safest.                                       |

## Approval Gates

Where a design, a fix approach or a plan needs explicit approval, present it in your output exactly as you would to your human partner, approve it yourself, and proceed. A committee's unresolved decisions are yours to settle: pick the position with the stronger evidence. In `debug-investigation`, a bug that cannot be reproduced is reported with what the evidence supports and left unfixed rather than guessed at.

## Stopping

Where a rule says to stop and consult, decide instead: choose the least destructive option that keeps the task moving. Stop only when going on would be destructive or impossible, and then end with a full report of what was done and what blocked you.

## Reporting

Your final message **MUST** list every decision you made in place of your human partner: each assumption, each routing choice, each self-approval, with a line on why. This is what lets a human check an unattended run afterwards.
