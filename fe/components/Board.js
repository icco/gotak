import React from "react";

import Square from './square.js';

export default class Board extends React.Component {
  render() {
    { size } = this.props

    if (size < 4 || size > 8) {
      return (<div>Not a valid board size.</div>);
    }

    const board = [];
    for(let i = 0; i < size; i++){
      const squareRows = [];
      for(let j = 0; j < size; j++){
        const squareShade = (i%2 && j%2) ? "light" : "dark";
        squareRows.push(this.renderSquare((i*size) + j, squareShade));
      }
      board.push(<div className="board-row">{squareRows}</div>)
    }

    return (
      <div>
        {board}
      </div>
    );
  }

  renderSquare(i, squareShade) {
    return (<Square
      shade = {squareShade}
      onClick={() => this.props.onClick(i)}
    />);
  }
}
