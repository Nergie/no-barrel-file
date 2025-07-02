export interface Level2Component {
  id: string;
  name: string;
}

export const createLevel2Component = (id: string, name: string): Level2Component => {
  return { id, name };
};
