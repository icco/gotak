import React from "react";

import "../css/square.scss";

export default function Square(props) {
  return (
    <div
      className={"square " + props.shade}
      onClick={props.onClick}
      style={props.style}
    />
  );
}
