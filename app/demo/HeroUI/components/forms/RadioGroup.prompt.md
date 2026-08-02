Mutually exclusive choice. The selected dot is drawn with a 6px inset ring, not a nested circle.

```jsx
<RadioGroup label="Plan" defaultValue="pro" onChange={setPlan}>
  <RadioGroup.Radio value="free" description="1 project">Free</RadioGroup.Radio>
  <RadioGroup.Radio value="pro" description="Unlimited projects">Pro</RadioGroup.Radio>
</RadioGroup>
```
