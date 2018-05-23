# gotak

[![Build Status](https://travis-ci.org/icco/gotak.svg?branch=master)](https://travis-ci.org/icco/gotak) [![GoDoc](https://godoc.org/github.com/icco/gotak?status.svg)](https://godoc.org/github.com/icco/gotak) [![Go Report Card](https://goreportcard.com/badge/github.com/icco/gotak)](https://goreportcard.com/report/github.com/icco/gotak)

A Tak server.

## Summary

The rough goal of this repo is to create a server that lets two people play tak together no matter their client. It needs to do the following things:

 - Parse PTN format
 - Provide a shared set of state between two players for a game
 - Store game history
 - Provide some sort of scoring?

## Inspirations

 - https://github.com/gruppler/PTN-Ninja
 - http://playtak.org
 - https://www.reddit.com/r/Tak/wiki/the_stacks
 - https://www.reddit.com/r/Tak/wiki/ptn_file_format
 - https://www.reddit.com/r/Tak/wiki/portable_tak_notation
 - http://blog.leahhanson.us/post/recursecenter2016/recipeparser.html
 - https://github.com/freeeve/pgn
 - https://github.com/nelhage/taktician

## TODO

  - Game Storage
    - Game state storage
        - Board
        - Pieces
        - Players
    - Game state history
        - Current state
        - Moves made
  - Game Play
    - Game move input
    - Game move validation
        - Game completion checking
    - Game notation parsing
  - Tests using real game notations

  - JSON API
     - GET `/game/$id`
         - Returns a game of ID `$id` with the most recent state.
     - GET `/game/$id/$move`
         - Returns a game of ID `$id` after move `$move`.
     - POST `/game/$id/move`
         - Client sends a move for game of `$id`. Request must include a valid player ID and a valid move.
     - POST `/game/new`
         - Creates a new game and returns a game ID. Must provide player IDs.

### Future

  - Scoring
  - Authentication
  - Player ranking
  - Sharing of game histories between friends
  - A gallery of well played games
  - A simple AI to play against if no one else is around
