---
name: research
description: Use when you need to conduct analysis or study something
---
# Research Skill

You MUST follow these instructions:

<research_instructions>

## Research

1. Review the list of previous researches in .claude/skills/research/index.md

  - Study researches relevant to the task.

2. If necessary, add an entry to the research index file
   .claude/skills/research/index.md

  - Format:

    ```md
    - [research name](.claude/skills/research/researches/<name>.md) —
      <brief description of no more than 20 words>
    ```

3. Create a new research file in .claude/skills/research/researches/<name>.md

  - Briefly describe the original question.
  - Make updates to the research file during the study process, not just at
    the final summary.

NOTICE: When delegating to an agent, require the research to be updated
incrementally, not all at once! Also, use code symbols instead of copying
structures or queries from the code.
</research_instructions>
