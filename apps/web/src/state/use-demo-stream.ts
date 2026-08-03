import { useContext } from 'react';

import { DemoStreamContext, type DemoStreamContextValue } from './demo-stream-context';

/** Access the DEMO platform store. Throws when used outside its provider. */
export function useDemoStream(): DemoStreamContextValue {
  const context = useContext(DemoStreamContext);
  if (context === null) {
    throw new Error('useDemoStream must be used inside <DemoStreamProvider>.');
  }
  return context;
}
