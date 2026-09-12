import { ActivityIndicator, Pressable, StyleSheet, Text, TextInput, View } from 'react-native';
import { tokens } from '@platform/ui';
import type { ApiClient, StopInput } from '@platform/api-client';
import type { Place } from '@platform/types';
import { usePlaceSearch } from '../features/places/usePlaceSearch';

/**
 * Well-known places, kept as a floor under the search box.
 *
 * Search can be unavailable — no geocoder configured, the provider down, the
 * phone offline — and a customer who cannot type an address should still be
 * able to book. These five are also faster than typing for the trips people
 * actually make most.
 *
 * There is still no map: a customer types or taps, rather than dropping a pin.
 */
export const PLACES: Array<{ name: string; stop: StopInput }> = [
  { name: 'Liberty Market', stop: { latitude: 31.5169, longitude: 74.3484 } },
  { name: 'Lahore Airport', stop: { latitude: 31.5216, longitude: 74.4036 } },
  { name: 'Emporium Mall', stop: { latitude: 31.4697, longitude: 74.2728 } },
  { name: 'Anarkali Bazaar', stop: { latitude: 31.5709, longitude: 74.3095 } },
  { name: 'Bahria Town', stop: { latitude: 31.3676, longitude: 74.1836 } },
];

/**
 * Choosing a point on the map without a map.
 *
 * Extracted from `BookingScreen` when grocery checkout became its second
 * consumer (CAP-6: extract a shared primitive when the second one appears, not
 * in anticipation of it). A ride needs a pickup; an order needs a door; both
 * are "name this place", and two copies of it would drift.
 */
export function PlacePicker({
  label,
  testID,
  selected,
  selectedName,
  disabled,
  client,
  near,
  placeholder,
  onSelect,
}: {
  label: string;
  testID: string;
  selected: StopInput | null;
  selectedName: string | null;
  disabled: boolean;
  client: ApiClient;
  near?: { latitude: number; longitude: number };
  placeholder?: string;
  /**
   * `address` is the fuller line a geocoder returned, where there was one.
   * A ride only needs the point; a delivery has to be written on a bag, and
   * "Liberty Market" is not a door.
   */
  onSelect(stop: StopInput | null, name: string | null, address?: string): void;
}) {
  const search = usePlaceSearch(client, near);

  const choose = (stop: StopInput, name: string, address?: string) => {
    onSelect(stop, name, address);
    search.clear();
  };

  return (
    <View style={styles.picker} testID={testID}>
      <Text style={styles.pickerLabel}>{label}</Text>

      {selected !== null ? (
        <Pressable
          testID={`${testID}-clear`}
          disabled={disabled}
          style={styles.selected}
          onPress={() => onSelect(null, null)}
        >
          <Text style={styles.selectedName}>{selectedName ?? 'Selected'}</Text>
          <Text style={styles.selectedChange}>Change</Text>
        </Pressable>
      ) : (
        <>
          <TextInput
            testID={`${testID}-input`}
            style={styles.input}
            value={search.query}
            editable={!disabled}
            placeholder={placeholder ?? 'Search for a place or address'}
            placeholderTextColor={tokens.color.textMuted}
            autoCorrect={false}
            onChangeText={search.setQuery}
          />

          {search.stage === 'searching' && (
            <ActivityIndicator testID={`${testID}-searching`} color={tokens.color.textMuted} />
          )}

          {/* An outage and an empty result must not read the same, or a
              customer retypes their address until they give up. */}
          {search.stage === 'failed' && (
            <Text style={styles.searchNotice} testID={`${testID}-search-error`}>
              {search.error}
            </Text>
          )}
          {search.stage === 'done' && search.results.length === 0 && (
            <Text style={styles.searchNotice} testID={`${testID}-no-results`}>
              No places matched. Try a different name.
            </Text>
          )}

          {search.results.map((place: Place, index: number) => (
            <Pressable
              key={`${place.name}-${index}`}
              testID={`${testID}-result-${index}`}
              disabled={disabled}
              style={styles.result}
              onPress={() =>
                choose(
                  { latitude: place.point.lat, longitude: place.point.lon },
                  place.name,
                  place.address,
                )
              }
            >
              <Text style={styles.resultName}>{place.name}</Text>
              <Text style={styles.resultAddress} numberOfLines={1}>
                {place.address}
              </Text>
            </Pressable>
          ))}

          {/* The floor: always reachable, whatever search is doing. */}
          {search.results.length === 0 && (
            <View style={styles.chips}>
              {PLACES.map((place) => (
                <Pressable
                  key={place.name}
                  testID={`${testID}-${place.name}`}
                  disabled={disabled}
                  style={styles.chip}
                  onPress={() => choose(place.stop, place.name)}
                >
                  <Text style={styles.chipText}>{place.name}</Text>
                </Pressable>
              ))}
            </View>
          )}
        </>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  picker: { marginBottom: tokens.space.lg },
  pickerLabel: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.sm,
  },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: tokens.space.sm },
  chip: {
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.lg,
    paddingVertical: tokens.space.sm,
    paddingHorizontal: tokens.space.md,
  },
  chipText: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  input: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    color: tokens.color.text,
    fontSize: tokens.fontSize.md,
    padding: tokens.space.sm,
    marginBottom: tokens.space.sm,
  },
  selected: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    padding: tokens.space.sm,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  selectedName: { color: tokens.color.text, fontSize: tokens.fontSize.md, flexShrink: 1 },
  selectedChange: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  result: {
    paddingVertical: tokens.space.sm,
    borderBottomColor: tokens.color.border,
    borderBottomWidth: 1,
  },
  resultName: { color: tokens.color.text, fontSize: tokens.fontSize.md },
  resultAddress: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  searchNotice: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    marginBottom: tokens.space.sm,
  },
});
