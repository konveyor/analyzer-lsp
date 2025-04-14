export interface Greeter {
  name: string;
  hello(): string;
}

export const greeter: Greeter = {
  name: "Person1",
  hello() {
    return `Hello, I'm ${this.name}`;
  },
};
