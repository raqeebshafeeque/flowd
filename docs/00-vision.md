# Vision - flowd

Why flowd exists and what winning looks like. Local only, one person, one day at
a time.

## Background

The day often lives in the head and in random notes. Opening a note app or a browser tab just to
plan something small breaks focus. By noon the plan goes stale and gets rebuilt from memory.

## Problem

1. Planning pulls a terminal developer out of the terminal where the work lives.
2. Tracking turns into managing tasks instead of doing them.
3. Review means digging through scattered notes to remember what really happened.

## Target user

A developer on Mac and Linux who lives in the terminal and likes keyboard over mouse. Planning
should take a couple minutes, tracking a keypress, review a few seconds.

Not for: teams, calendar sync fans, phone first folks, or anyone needing pings while the app stays
closed.

## Goals

- G1: One glance shows what runs now and what comes next.
- G2: Every move costs one key: start, pause, finish, skip and stretch.
- G3: Night review fits one screen: a score plus missed and long runners.
- G4: Everything stays on the machine, works offline and opens instantly.

## Non-goals

- No sharing, no sync, nothing over the network.
- No background pings while the app stays closed.
- The full cut list sits in [requirements.md](01-requirements.md) under out of scope.

## Vision statement

flowd keeps a developer in flow. Plan fast, track with keys and look back in one glance.

## Success criteria

- S1: Daily use for 30 days for work and study with no slip back to random notes and tabs.
- S2: Planning feels quick and review reads in seconds through the 30 day trial. Judged by feel in
  daily use, since the app sends no telemetry.
- S3: No lost days across kill and reopen during those 30 days.

## Constraints

- Terminal first on Mac and Linux.
- One local install, data in a local file only.
- Local time on screen, UTC underneath.
