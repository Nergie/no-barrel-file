export type DeepType = {
  level: number;
  nested: {
    value: string;
    metadata: {
      created: Date;
      author: string;
    };
  };
};

export enum DeepEnum {
  LEVEL_ONE = "level_1",
  LEVEL_TWO = "level_2", 
  LEVEL_THREE = "level_3",
}
