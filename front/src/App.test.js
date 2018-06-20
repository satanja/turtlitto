import React from "react";
import { Server } from "mock-socket";
import { shallow } from "enzyme";
import App from "./App";

jest.useFakeTimers();

/* 
 * Test_items: App.js
 * Input_spec: -
 * Output_spec: -
 * Envir_needs: The User has logged in and has enabled a TURTLE.
 */
describe("App.js", () => {
  it("automatically reconnects", () => {
    //Establish a server and wait for a connection
    const l = window.location;
    const mockServer = new Server(`ws://${l.host}/api/v1/state`);
    let connectionCount = 0;
    mockServer.on("connection", server => {
      console.log(server);
      connectionCount++;
    });
    mockServer.on("error", server => {
      console.log(server);
    });
    mockServer.on("close", server => {
      console.log(server);
      connectionCount--;
    });
    const wrapper = shallow(<App />);
    wrapper.setState({ session: "testtoken" });
    wrapper.instance().connect();
    //Let the reconnect time run down
    jest.runAllTimers();
    //Check if exactly one connection has been made
    expect(connectionCount).toBe(1);
    //Close the server to disconnect and check if there is no connection
    mockServer.close();
    expect(connectionCount).toBe(0);
    //Establish a new server and wait for a connection
    const mockServer2 = new Server(`ws://${l.host}/api/v1/state`);
    mockServer2.on("connection", server => {
      connectionCount++;
    });
    mockServer2.on("close", server => {
      connectionCount--;
    });
    //jest.runAllTimers();
    //Wait one second since it will take one second to reconnect
    setTimeout(() => {
      //Check if exactly one connection has been made
      expect(connectionCount).toBe(1);
      wrapper.unmount(this);
    }, 1000);
    jest.runAllTimers();
    mockServer2.close();
  });

  it("renders without crashing", () => {
    shallow(<App />);
  });

  describe("updates the local state when it", () => {
    let serverMessage, expectedState, initialTurtles;
    const l = window.location;
    afterEach(() => {
      const mockServer = new Server(`ws://${l.host}/api/v1/state`);
      mockServer.on("connection", server => {
        mockServer.send(serverMessage);
      });
      mockServer.on("error", server => {
        console.log(server);
      });
      const wrapper = shallow(<App />);
      wrapper.setState({ token: "testtoken", turtles: initialTurtles });
      wrapper.instance().connect();
      /*
       * Needed to make App.js connect to the mock server.
       * If you use runAllTimers, it goes in a loop, if you run it <3 times, it doesn't work.
       */
      jest.runOnlyPendingTimers();
      jest.runOnlyPendingTimers();
      jest.runOnlyPendingTimers();
      expect(JSON.stringify(wrapper.state("turtles"))).toBe(expectedState);
      mockServer.close();
    });

    it("gets new turtles", () => {
      initialTurtles = {};
      serverMessage = '{"turtles": {"1":{"battery": 99}}}';
      expectedState = '{"1":{"battery":99,"enabled":false}}';
    });

    it("gets a turtle update", () => {
      initialTurtles = { 2: { battery: 88, enabled: false } };
      serverMessage =
        '{"turtles": {"2":{"battery": 87, "teamcolor": "magenta"}}}';
      expectedState =
        '{"2":{"battery":87,"enabled":false,"teamcolor":"magenta"}}';
    });

    it("gets nothing new at all", () => {
      initialTurtles = { 1: { battery: 77, enabled: true } };
      serverMessage = "{}\n";
      expectedState = '{"1":{"battery":77,"enabled":true}}';
    });
  });
});
