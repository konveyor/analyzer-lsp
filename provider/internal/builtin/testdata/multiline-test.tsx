import React from 'react';
import { Masthead, MastheadToggle, MastheadBrand } from '@patternfly/react-core';

const App: React.FC = () => {
  return (
    <div>
      {/* This should match - multiline JSX */}
      <Masthead>
        <MastheadToggle>Foo</MastheadToggle>
        <MastheadBrand>Bar</MastheadBrand>
      </Masthead>

      {/* This should also match - single line */}
      <Masthead><MastheadToggle>Test</MastheadToggle></Masthead>

      {/* This should match - multiline with lots of whitespace */}
      <Masthead>


        <MastheadBrand>Baz</MastheadBrand>
      </Masthead>

      {/* This should NOT match - no child elements */}
      <Masthead>
      </Masthead>
    </div>
  );
};

export default App;
