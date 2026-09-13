import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { useState } from 'react';
import { formatMoney, tokens } from '@platform/ui';
import type { Money, Product, ProductVariant, SubstitutionPreference } from '@platform/types';
import type { GroceryActions, GroceryState } from '../features/grocery/useGrocery';

/**
 * What to do when the shelf is empty (document 74).
 *
 * Asked per line, at the moment the line is added, because that is the only
 * moment the customer is thinking about this particular item. `ASK_ME` is the
 * default the server applies when nothing is sent; it is offered explicitly
 * here so the choice is visible rather than inherited.
 */
const PREFERENCES: Array<{ value: SubstitutionPreference; label: string }> = [
  { value: 'ALLOW', label: 'Any similar' },
  { value: 'ASK_ME', label: 'Ask me' },
  { value: 'DO_NOT_ALLOW', label: 'Skip it' },
];

export function ShopScreen({ grocery }: { grocery: GroceryState & GroceryActions }) {
  const cart = grocery.order;
  const lines = cart?.items ?? [];
  const empty = lines.length === 0;

  return (
    <ScrollView contentContainerStyle={styles.screen} testID="shop-screen">
      <Pressable testID="shop-back" onPress={grocery.reset}>
        <Text style={styles.back}>‹ All shops</Text>
      </Pressable>
      <Text style={styles.title}>{grocery.store?.merchant_name ?? 'Shop'}</Text>

      {grocery.error !== null && (
        <Text style={styles.error} testID="shop-error">
          {grocery.error}
        </Text>
      )}

      {grocery.products.map((product) => (
        <ProductRow
          key={product.id}
          product={product}
          disabled={grocery.pending}
          onAdd={(quantity, preference, variantId) =>
            void (variantId
              ? grocery.addItem(product, quantity, preference, variantId)
              : grocery.addItem(product, quantity, preference))
          }
        />
      ))}

      {grocery.pending && <ActivityIndicator testID="shop-pending" color={tokens.color.accent} />}

      <View style={styles.cart} testID="cart-summary">
        <Text style={styles.cartTitle}>Your basket</Text>
        {empty ? (
          <Text style={styles.cartEmpty} testID="cart-empty">
            Nothing in it yet.
          </Text>
        ) : (
          lines.map((line) => (
            <View key={line.id} style={styles.cartRow} testID={`cart-line-${line.id}`}>
              <Text style={styles.cartLine} numberOfLines={1}>
                {line.quantity} × {line.name}
              </Text>
              <Text style={styles.cartAmount}>{formatMoney(line.line_total)}</Text>
            </View>
          ))
        )}
        {cart && !empty && (
          <View style={[styles.cartRow, styles.cartTotalRow]}>
            <Text style={styles.cartTotalLabel}>Items</Text>
            <Text style={styles.cartTotalLabel} testID="cart-total">
              {formatMoney(cart.items_total)}
            </Text>
          </View>
        )}
        {/* Delivery is priced by the server at checkout, and this screen has
            not asked where it is going yet, so no total is promised here. */}
      </View>

      <Pressable
        testID="shop-checkout"
        // An empty cart is refused by the server with "there is nothing in
        // this cart yet". Refusing it here saves the round trip and the
        // moment of doubt.
        disabled={empty || grocery.pending}
        style={[styles.button, (empty || grocery.pending) && styles.buttonDisabled]}
        onPress={grocery.toCheckout}
      >
        <Text style={styles.buttonText}>Checkout</Text>
      </Pressable>
    </ScrollView>
  );
}

/**
 * A variant's own price: document 68 stores the difference, not the price.
 *
 * Computed here for display only. What the customer is charged is the line the
 * server writes from its own catalogue, and this must never be the number the
 * order is built from — it exists so a customer can see what the 2L costs
 * before they choose it.
 */
function variantPrice(base: Money, variant: ProductVariant): Money {
  return { ...base, amount_minor: base.amount_minor + variant.price_diff.amount_minor };
}

/**
 * Which variant a row opens on.
 *
 * The first one that is actually in stock, so a shop whose 1L has run out
 * opens on the 2L rather than on a selection that cannot be added. Null when
 * the product has no variants at all, which is most of a kiryana's shelf.
 */
function firstSellable(product: Product): ProductVariant | null {
  const variants = product.variants ?? [];
  return variants.find((variant) => variant.available) ?? variants[0] ?? null;
}

function ProductRow({
  product,
  disabled,
  onAdd,
}: {
  product: Product;
  disabled: boolean;
  onAdd(quantity: number, preference: SubstitutionPreference, variantId?: string): void;
}) {
  const [quantity, setQuantity] = useState(1);
  const [preference, setPreference] = useState<SubstitutionPreference>('ASK_ME');
  const variants = product.variants ?? [];
  const [variantId, setVariantId] = useState<string | null>(
    () => firstSellable(product)?.id ?? null,
  );
  const variant = variants.find((option) => option.id === variantId) ?? null;

  // A product whose every size has run out is out of stock, whatever the
  // product row says: document 69 tracks availability per variant, and the
  // parent's flag is about the default line.
  const sellable =
    variants.length > 0 ? variants.some((option) => option.available) : product.available;

  if (!sellable) {
    return (
      <View style={styles.product} testID={`product-${product.id}`}>
        <Text style={styles.productName}>{product.name}</Text>
        <Text style={styles.unavailable}>Out of stock</Text>
      </View>
    );
  }

  return (
    <View style={styles.product} testID={`product-${product.id}`}>
      <View style={styles.productHead}>
        <Text style={styles.productName} numberOfLines={1}>
          {product.name}
        </Text>
        <Text style={styles.productPrice} testID={`product-${product.id}-price`}>
          {formatMoney(variant ? variantPrice(product.price, variant) : product.price)}
        </Text>
      </View>
      {product.description ? (
        <Text style={styles.productDescription} numberOfLines={2}>
          {product.description}
        </Text>
      ) : null}

      {variants.length > 0 && (
        <View style={styles.chips} testID={`product-${product.id}-variants`}>
          {variants.map((option) => (
            <Pressable
              key={option.id}
              testID={`product-${product.id}-variant-${option.id}`}
              style={[styles.chip, option.id === variantId && styles.chipActive]}
              // An option nobody can be sold is shown and not selectable.
              // Hiding it tells a customer the shop does not stock the 2L at
              // all, when the truth is that it is out today.
              disabled={disabled || !option.available}
              onPress={() => setVariantId(option.id)}
            >
              <Text style={option.id === variantId ? styles.chipTextActive : styles.chipText}>
                {option.available ? option.name : `${option.name} — out`}
              </Text>
            </Pressable>
          ))}
        </View>
      )}

      <View style={styles.chips}>
        {PREFERENCES.map((option) => (
          <Pressable
            key={option.value}
            testID={`product-${product.id}-${option.value}`}
            style={[styles.chip, option.value === preference && styles.chipActive]}
            disabled={disabled}
            onPress={() => setPreference(option.value)}
          >
            <Text style={option.value === preference ? styles.chipTextActive : styles.chipText}>
              {option.label}
            </Text>
          </Pressable>
        ))}
      </View>

      <View style={styles.productFoot}>
        <View style={styles.stepper}>
          <Pressable
            testID={`product-${product.id}-less`}
            style={styles.step}
            disabled={disabled || quantity <= 1}
            onPress={() => setQuantity((q) => Math.max(1, q - 1))}
          >
            <Text style={styles.stepText}>−</Text>
          </Pressable>
          <Text style={styles.quantity} testID={`product-${product.id}-quantity`}>
            {quantity}
          </Text>
          <Pressable
            testID={`product-${product.id}-more`}
            style={styles.step}
            disabled={disabled}
            onPress={() => setQuantity((q) => q + 1)}
          >
            <Text style={styles.stepText}>+</Text>
          </Pressable>
        </View>
        <Pressable
          testID={`product-${product.id}-add`}
          style={styles.add}
          disabled={disabled}
          // A product with no variants sends no variant, rather than sending
          // one that is undefined: the call a kiryana's shelf makes is
          // unchanged by this screen learning about sizes.
          onPress={() =>
            variantId ? onAdd(quantity, preference, variantId) : onAdd(quantity, preference)
          }
        >
          <Text style={styles.addText}>Add</Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { backgroundColor: tokens.color.background, padding: tokens.space.lg, flexGrow: 1 },
  back: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  title: {
    color: tokens.color.text,
    fontSize: tokens.fontSize.xl,
    fontWeight: '600',
    marginBottom: tokens.space.lg,
  },
  product: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    marginBottom: tokens.space.sm,
  },
  productHead: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'baseline' },
  productName: { color: tokens.color.text, fontSize: tokens.fontSize.md, flexShrink: 1 },
  productPrice: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  productDescription: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  unavailable: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  productFoot: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginTop: tokens.space.sm,
  },
  stepper: { flexDirection: 'row', alignItems: 'center', gap: tokens.space.md },
  step: {
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.sm,
    paddingHorizontal: tokens.space.md,
    paddingVertical: tokens.space.xs,
  },
  stepText: { color: tokens.color.text, fontSize: tokens.fontSize.md },
  quantity: { color: tokens.color.text, fontSize: tokens.fontSize.md, minWidth: 20 },
  add: {
    backgroundColor: tokens.color.accent,
    borderRadius: tokens.radius.sm,
    paddingHorizontal: tokens.space.lg,
    paddingVertical: tokens.space.sm,
  },
  addText: { color: tokens.color.text, fontSize: tokens.fontSize.sm, fontWeight: '600' },
  chips: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: tokens.space.sm,
    marginTop: tokens.space.sm,
  },
  chip: {
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.lg,
    paddingVertical: tokens.space.xs,
    paddingHorizontal: tokens.space.md,
  },
  chipActive: { backgroundColor: tokens.color.accent, borderColor: tokens.color.accent },
  chipText: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  chipTextActive: { color: tokens.color.text, fontSize: tokens.fontSize.sm, fontWeight: '600' },
  cart: {
    borderTopColor: tokens.color.border,
    borderTopWidth: 1,
    marginTop: tokens.space.lg,
    paddingTop: tokens.space.md,
  },
  cartTitle: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  cartEmpty: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  cartRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    paddingVertical: tokens.space.xs,
  },
  cartLine: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm, flexShrink: 1 },
  cartAmount: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  cartTotalRow: {
    borderTopColor: tokens.color.border,
    borderTopWidth: 1,
    marginTop: tokens.space.xs,
    paddingTop: tokens.space.sm,
  },
  cartTotalLabel: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  button: {
    backgroundColor: tokens.color.accent,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
    marginTop: tokens.space.lg,
  },
  buttonDisabled: { opacity: 0.5 },
  buttonText: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  error: {
    color: tokens.color.danger,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.md,
  },
});
