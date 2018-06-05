import React from "react";
import PropTypes from "prop-types";
import Turtle from "../Turtle";
import styled from "styled-components";
import Dropdown from "../Dropdown";
import sendToServer from "../sendToServer";

/**
 * Show the settings of all turtles.
 * Author: H.E. van der Laan
 * Author: T.T.P. Franken
 *
 * props:
 * - turtles: an array of Turtles
 * - onTurtleEnableChange: function to call when the turtle enable button is pressed
 * - onSettingsChange: a function which dictates what happens when a dropdown is changed.
 */

const CONFIG_VALUES = [
  "Role assigner on",
  "Role assigner off",
  "Pass demo",
  "Penalty mode",
  "Ball Handling demo"
];

const COMMAND_VALUES = {
  "Role assigner on": "role_assigner_on",
  "Role assigner off": "role_assigner_off",
  "Pass demo": "pass_demo",
  "Penalty mode": "penalty_demo",
  "Ball Handling demo": "ball_handling_demo"
};

const Settings = props => {
  const { turtles } = props;
  return (
    <SettingsWrapper>
      <RoleDropdown
        id={"settings_role-dropdown"}
        currentValue={"Whatever"}
        values={CONFIG_VALUES}
        onChange={value => {
          sendToServer(COMMAND_VALUES[value], "command");
        }}
        enabled={true}
      />
      {turtles.map(turtle => (
        <Turtle
          key={turtle.id}
          turtle={turtle}
          editable
          onChange={(changedProp, newValue) => {} /* TODO: turtle update */}
        />
      ))}
    </SettingsWrapper>
  );
};

Settings.propTypes = {
  turtles: PropTypes.arrayOf(
    PropTypes.shape({
      id: PropTypes.number.isRequired,
      enabled: PropTypes.bool.isRequired,
      batteryvoltage: PropTypes.number.isRequired,
      homegoal: PropTypes.string.isRequired,
      role: PropTypes.string.isRequired,
      teamcolor: PropTypes.string.isRequired
    })
  ).isRequired
};

const RoleDropdown = styled(Dropdown)`
  width: 75%;
  margin-left: auto;
  margin-right: auto;
  height: 4rem;
`;

const SettingsWrapper = styled.div`
  display: flex;
  flex-direction: column;
`;

export default Settings;
